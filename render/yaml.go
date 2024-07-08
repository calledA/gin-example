// Copyright 2014 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package render

import (
	"net/http"

	"gopkg.in/yaml.v3"
)

// YAML 结构体
type YAML struct {
	Data any
}

// yaml的ContentType
var yamlContentType = []string{"application/x-yaml; charset=utf-8"}

// Render YAML数据
func (r YAML) Render(w http.ResponseWriter) error {
	// 先将yamlContentType写入header的Content-Type
	r.WriteContentType(w)

	// r.Data进行yml.Marshal转义
	bytes, err := yaml.Marshal(r.Data)
	if err != nil {
		return err
	}

	// 写入bytes数据
	_, err = w.Write(bytes)
	return err
}

// 将yamlContentType写入header的Content-Type
func (r YAML) WriteContentType(w http.ResponseWriter) {
	writeContentType(w, yamlContentType)
}
