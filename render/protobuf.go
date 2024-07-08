// Copyright 2018 Gin Core Team. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package render

import (
	"net/http"

	"google.golang.org/protobuf/proto"
)

// ProtoBuf 结构体
type ProtoBuf struct {
	Data any
}

// protobuf的ContentType
var protobufContentType = []string{"application/x-protobuf"}

// Render ProtoBuf数据
func (r ProtoBuf) Render(w http.ResponseWriter) error {
	// 先将protobufContentType写入header的Content-Type
	r.WriteContentType(w)

	// r.Data进行proto.Marshal转义
	bytes, err := proto.Marshal(r.Data.(proto.Message))
	if err != nil {
		return err
	}

	// 写入bytes数据
	_, err = w.Write(bytes)
	return err
}

// 将protobufContentType写入header的Content-Type
func (r ProtoBuf) WriteContentType(w http.ResponseWriter) {
	writeContentType(w, protobufContentType)
}
