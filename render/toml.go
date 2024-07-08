// Copyright 2022 Gin Core Team. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package render

import (
	"net/http"

	"github.com/pelletier/go-toml/v2"
)

// TOML 结构体
type TOML struct {
	Data any
}

// toml的Content-Type
var TOMLContentType = []string{"application/toml; charset=utf-8"}

// Render TOML数据
func (r TOML) Render(w http.ResponseWriter) error {
	// 先将TOMLContentType写入header的Content-Type
	r.WriteContentType(w)

	// r.Data进行toml.Marshal转义
	bytes, err := toml.Marshal(r.Data)
	if err != nil {
		return err
	}

	// 写入bytes数据
	_, err = w.Write(bytes)
	return err
}

// 将TOMLContentType写入header的Content-Type
func (r TOML) WriteContentType(w http.ResponseWriter) {
	writeContentType(w, TOMLContentType)
}
