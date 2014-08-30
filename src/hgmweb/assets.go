/*
 * Copyright (C) 2013-2014 Adrian Ulrich <adrian@blinkenlights.ch>
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program. If not, see <http://www.gnu.org/licenses/>.
 */

package hgmweb
import (
	"fmt"
	"io"
	"net/http"
)

type assetContent struct {
	ContentType string
	Blob        string
}

func serveAsset(w http.ResponseWriter, assetName string) {

	httpStatus := http.StatusOK
	asset := emptyText();

	switch assetName {
		case "basic.css":
			asset = baseCSS()
		default:
			httpStatus = http.StatusNotFound
	}

	fmt.Printf("name=%s, status=%d\n", assetName, httpStatus);
	fmt.Printf(">> %s\n", assetName);

	w.Header().Set("Content-Type", asset.ContentType)
	w.WriteHeader(httpStatus)
	io.WriteString(w, asset.Blob)
}

/**
 * Returns the wwwpath to given asset name
 */
func getAssetPath(assetName string, config *proxyParams) string {
	return fmt.Sprintf("/%s%s%s", config.Webroot, config.Assets, assetName)
}

/**
 * Returns an empty string
 */
func emptyText() *assetContent {
	return &assetContent {
		ContentType: "text/plain",
		Blob: "",
	}
}

/**
 * Returns our basic CSS
 */
func baseCSS() *assetContent {
	return &assetContent {
		ContentType: "text/html",
		Blob: "funkyShit",
	}
}
