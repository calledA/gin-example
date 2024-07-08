// Copyright 2014 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package render

import (
	"encoding/xml"
	"net/http"
)

// XML 结构体
type XML struct {
	Data any
}

// xml的ContentType
var xmlContentType = []string{"application/xml; charset=utf-8"}

// Render XML数据
func (r XML) Render(w http.ResponseWriter) error {
	// 先将protobufContentType写入header的ContentType
	r.WriteContentType(w)
	// 新建一个xml的encoder，encode过程中会调用w.Write进行echo数据
	return xml.NewEncoder(w).Encode(r.Data)
}

// 将protobufContentType写入header的ContentType
func (r XML) WriteContentType(w http.ResponseWriter) {
	writeContentType(w, xmlContentType)
}
