/**
 * Copyright (c) 2014 Deepin, Inc.
 *               2014 Xu FaSheng
 *
 * Author:      Xu FaSheng <fasheng.xu@gmail.com>
 * Maintainer:  Xu FaSheng <fasheng.xu@gmail.com>
 *
 * This program is free software; you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation; either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program; if not, see <http://www.gnu.org/licenses/>.
 **/

package main

import (
	"dlib/gio-2.0"
	"os"
)

const (
	envHttpProxy  = "http-proxy"
	envHttpsProxy = "https-proxy"
	envFtpProxy   = "ftp-proxy"
	envSocksProxy = "socks-proxy"

	gsettingsIdProxy = "com.deepin.dde.proxy"
	gkeyHttpProxy    = envHttpProxy
	gkeyHttpsProxy   = envHttpsProxy
	gkeyFtpProxy     = envFtpProxy
	gkeySocksProxy   = envSocksProxy
)

var (
	proxySettings = gio.NewSettings(gsettingsIdProxy)
)

func startProxy() {
	updateProxyEnvs()
	listenProxyGsettings()
}

func updateProxyEnvs() {
	httpProxy := proxySettings.GetString(gkeyHttpProxy)
	os.Setenv(envHttpProxy, httpProxy)

	httpsProxy := proxySettings.GetString(gkeyHttpsProxy)
	os.Setenv(envHttpsProxy, httpsProxy)

	ftpProxy := proxySettings.GetString(gkeyFtpProxy)
	os.Setenv(envFtpProxy, ftpProxy)

	socksProxy := proxySettings.GetString(gkeySocksProxy)
	os.Setenv(envSocksProxy, socksProxy)
}

func listenProxyGsettings() {
	proxySettings.Connect("changed", func(s *gio.Settings, key string) {
		Logger.Debug("proxy value in gsettings changed:", key, s.GetString(key))
		updateProxyEnvs()
	})
}
