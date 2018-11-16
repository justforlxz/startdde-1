/*
 * Copyright (C) 2017 ~ 2018 Deepin Technology Co., Ltd.
 *
 * Author:     jouyouyun <jouyouwen717@gmail.com>
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package wm

import (
	"errors"
	"os/exec"
	"strings"
	"sync"

	libwm "dbus/com/deepin/wm"

	"github.com/linuxdeepin/go-x11-client"
	"github.com/linuxdeepin/go-x11-client/util/wm/ewmh"
	"pkg.deepin.io/lib/dbus"
	"pkg.deepin.io/lib/log"
)

const (
	swDBusDest = "com.deepin.WMSwitcher"
	swDBusPath = "/com/deepin/WMSwitcher"
	swDBusIFC  = swDBusDest

	deepin3DWM = "deepin-wm"
	deepin2DWM = "deepin-metacity"
	unknownWM  = "unknown"

	osdSwitch2DWM    = "SwitchWM2D"
	osdSwitch3DWM    = "SwitchWM3D"
	osdSwitchWMError = "SwitchWMError"
)

var wmNameMap = map[string]string{
	deepin3DWM: "deepin wm",
	deepin2DWM: "deepin metacity",
}

//Switcher wm switch manager
type Switcher struct {
	logger       *log.Logger
	userConfig   *userConfig
	systemConfig *systemConfig
	mu           sync.Mutex

	wm                *libwm.Wm
	wmStartupCount    int
	currentWM         string
	WMChanged         func(string)
	wmChooserLaunched bool
}

func (s *Switcher) allowSwitch() bool {
	return s.systemConfig.AllowSwitch
}

func (s *Switcher) getWM() string {
	if s.allowSwitch() {
		return s.userConfig.LastWM
	}
	return deepin2DWM
}

func (s *Switcher) shouldWait() bool {
	if s.allowSwitch() {
		return s.userConfig.Wait
	}
	return true
}

func (s *Switcher) setCurrentWM(name string) {
	s.mu.Lock()
	if s.currentWM != name {
		s.currentWM = name
		s.emitSignalWMChanged(name)
	}
	s.mu.Unlock()
}

// CurrentWM show the current window manager
func (s *Switcher) CurrentWM() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	wmName := wmNameMap[s.currentWM]
	if wmName == "" {
		return "unknown"
	}
	return wmName
}

// RestartWM restart the last window manager
func (s *Switcher) RestartWM() error {
	return s.runWM(s.getWM(), true)
}

// Start2DWM called by startdde watchdog, run 2d window manager without --replace option.
func (s *Switcher) Start2DWM() error {
	err := s.runWM(deepin2DWM, false)
	if err != nil {
		return err
	}

	// NOTE: Don't save to config file.
	s.setLastWM(deepin2DWM)
	return nil
}

// RequestSwitchWM try to switch window manager
func (s *Switcher) RequestSwitchWM() error {
	if !s.allowSwitch() {
		showOSD(osdSwitchWMError)
		return errors.New("refused to switch wm")
	}

	s.mu.Lock()
	currentWM := s.currentWM
	s.mu.Unlock()

	nextWM := deepin3DWM
	if currentWM == deepin3DWM {
		nextWM = deepin2DWM
	} else if currentWM == deepin2DWM {
		nextWM = deepin3DWM
	}

	err := s.runWM(nextWM, true)
	if err != nil {
		return err
	}

	s.setLastWM(nextWM)
	s.saveUserConfig()
	s.adjustSogouSkin()
	return nil
}

//GetDBusInfo return dbus object info
func (*Switcher) GetDBusInfo() dbus.DBusInfo {
	return dbus.DBusInfo{
		Dest:       swDBusDest,
		ObjectPath: swDBusPath,
		Interface:  swDBusIFC,
	}
}

func (s *Switcher) emitSignalWMChanged(wm string) {
	dbus.Emit(s, "WMChanged", wmNameMap[wm])
}

func (s *Switcher) isCardChanged() (change bool) {
	actualCardInfos, err := getCardInfos()
	if err != nil {
		s.logger.Warning("failed to get card info:", err)
		return true
	}
	actualCardInfosStr := actualCardInfos.String()
	s.logger.Debug("actualCardInfos:", actualCardInfosStr)

	cacheCardInfos, err := loadCardInfosFromFile(getCardInfosPath())
	if err != nil {
		s.logger.Warning("failed to load card info from config file:", err)
		change = true
	} else {
		// load cacheCardInfos ok
		cacheCardInfosStr := cacheCardInfos.String()
		s.logger.Debug("cacheCardInfos:", cacheCardInfosStr)
		if actualCardInfosStr != cacheCardInfosStr {
			// card changed
			change = true
		}
	}

	if change {
		err = doSaveCardInfos(getCardInfosPath(), actualCardInfos.genCardConfig())
		if err != nil {
			s.logger.Warning("failed to save card infos:", err)
		}
	}

	return change
}

func (s *Switcher) init() {
	s.loadSystemConfig()

	if s.allowSwitch() {
		cardChanged := s.isCardChanged()
		if !s.wmChooserLaunched && cardChanged {
			s.initUserConfig()
		} else {
			err := s.loadUserConfig()
			if err != nil {
				s.initUserConfig()
			}
		}
	}
}

func (s *Switcher) listenStartupReady() {
	var err error
	s.wm, err = libwm.NewWm("com.deepin.wm", "/com/deepin/wm")
	if err != nil {
		panic(err)
	}

	s.wm.ConnectStartupReady(func(wmName string) {
		s.mu.Lock()
		count := s.wmStartupCount
		s.wmStartupCount++
		s.mu.Unlock()
		s.logger.Debug("receive signal StartupReady", wmName, count)

		if count > 0 {
			switch wmName {
			case deepin3DWM:
				showOSD(osdSwitch3DWM)
			case deepin2DWM:
				showOSD(osdSwitch2DWM)
			}
		}
	})
}

func (s *Switcher) listenWMChanged() {
	s.currentWM = s.getWM()

	conn, err := x.NewConn()
	if err != nil {
		s.logger.Warning(err)
		return
	}

	root := conn.GetDefaultScreen().Root
	err = x.ChangeWindowAttributesChecked(conn, root, x.CWEventMask, []uint32{
		x.EventMaskPropertyChange}).Check(conn)
	if err != nil {
		s.logger.Warning(err)
		return
	}

	atomNetSupportingWMCheck, err := conn.GetAtom("_NET_SUPPORTING_WM_CHECK")
	if err != nil {
		s.logger.Warning(err)
		return
	}

	eventChan := make(chan x.GenericEvent, 10)
	conn.AddEventChan(eventChan)

	handlePropNotifyEvent := func(event *x.PropertyNotifyEvent) {
		if event.Atom != atomNetSupportingWMCheck || event.Window != root {
			return
		}
		switch event.State {
		case x.PropertyNewValue:
			win, err := ewmh.GetSupportingWMCheck(conn).Reply(conn)
			if err != nil {
				s.logger.Warning(err)
				return
			}
			s.logger.Debug("win:", win)

			wmName, err := ewmh.GetWMName(conn, win).Reply(conn)
			if err != nil {
				s.logger.Warning(err)
				return
			}
			s.logger.Debug("wmName:", wmName)

			var currentWM string
			if wmName == "Metacity" {
				currentWM = deepin2DWM
			} else if strings.Contains(wmName, "DeepinGala") {
				currentWM = deepin3DWM
			} else {
				currentWM = unknownWM
			}
			s.setCurrentWM(currentWM)

		case x.PropertyDelete:
			s.logger.Debug("wm lost")
		}
	}

	go func() {
		for ev := range eventChan {
			switch ev.GetEventCode() {
			case x.PropertyNotifyEventCode:
				event, _ := x.NewPropertyNotifyEvent(ev)
				handlePropNotifyEvent(event)
			}
		}
	}()
}

func (s *Switcher) adjustSogouSkin() {
	filename := getSogouConfigPath()
	skin, _ := getSogouSkin(filename)
	if skin != "" && s.getWM() == deepin3DWM {
		return
	}

	if skin == sgDefaultSkin {
		return
	}

	err := setSogouSkin(sgDefaultSkin, filename)
	if err != nil {
		s.logger.Warning("Failed to set sogou skin:", err)
		return
	}

	outs, err := exec.Command("killall", "sogou-qimpanel").CombinedOutput()
	if err != nil {
		s.logger.Warning("Failed to terminate sogou:", string(outs), err)
	}
}

func (s *Switcher) initUserConfig() {
	if s.supportRunGoodWM() {
		s.userConfig = &userConfig{
			LastWM: deepin3DWM,
			Wait:   true,
		}
	} else {
		s.userConfig = &userConfig{
			LastWM: deepin2DWM,
			Wait:   true,
		}
	}

	s.saveUserConfig()
}

func (s *Switcher) runWM(wm string, replace bool) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return err
	}

	args := []string{"GDK_SCALE=1", wm}
	if replace {
		args = append(args, "--replace")
	}
	obj := conn.Object("com.deepin.SessionManager", "/com/deepin/StartManager")
	err = obj.Call("com.deepin.StartManager.RunCommand", 0,
		"env", args).Store()
	if err != nil {
		return err
	}
	return nil
}

func (s *Switcher) supportRunGoodWM() bool {
	support := true

	platform, err := getPlatform()
	if err == nil && platform == platformSW {
		if !isRadeonExists() {
			support = false
			setupSWPlatform()
		}
	}

	if !isDriverLoadedCorrectly() {
		support = false
		return support
	}

	env, err := getVideoEnv()
	if err == nil {
		correctWMByEnv(env, &support)
	}

	return support
}
