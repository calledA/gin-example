// Copyright 2014 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

//go:build !nomsgpack

package binding

import "net/http"

// 常见的Content-Type类型
const (
	MIMEJSON              = "application/json"
	MIMEHTML              = "text/html"
	MIMEXML               = "application/xml"
	MIMEXML2              = "text/xml"
	MIMEPlain             = "text/plain"
	MIMEPOSTForm          = "application/x-www-form-urlencoded"
	MIMEMultipartPOSTForm = "multipart/form-data"
	MIMEPROTOBUF          = "application/x-protobuf"
	MIMEMSGPACK           = "application/x-msgpack"
	MIMEMSGPACK2          = "application/msgpack"
	MIMEYAML              = "application/x-yaml"
	MIMETOML              = "application/toml"
)

// 提供参数绑定的接口，不同的Content-Type实现该接口，实现对应的处理
type Binding interface {
	Name() string
	Bind(*http.Request, any) error
}

// 继承Binding接口之外，BindBody提供了[]byte类型的参数绑定函数接口
type BindingBody interface {
	// 添加Binding接口方法
	Binding
	// BindBody通过bytes绑定而不是req.Body
	BindBody([]byte, any) error
}

// 提供Params参数绑定的接口
type BindingUri interface {
	Name() string
	BindUri(map[string][]string, any) error
}

// StructValidator提供最小的函数接口，用来实现validator，确保请求的正确性
type StructValidator interface {
	// ValidateStruct接受any类型，但是只会处理结构体、指针和指向指针的类型（Slice和Array）
	ValidateStruct(any) error

	// 返回StructValidator的底层validator引擎
	Engine() any
}

// defaultValidator是默认的validator，实现了StructValidator接口
var Validator StructValidator = &defaultValidator{}

// 实现了Binding接口用来绑定数据
var (
	JSON          = jsonBinding{}
	XML           = xmlBinding{}
	Form          = formBinding{}
	Query         = queryBinding{}
	FormPost      = formPostBinding{}
	FormMultipart = formMultipartBinding{}
	ProtoBuf      = protobufBinding{}
	MsgPack       = msgpackBinding{}
	YAML          = yamlBinding{}
	Uri           = uriBinding{}
	Header        = headerBinding{}
	TOML          = tomlBinding{}
)

// 根据request方法和content-type来返回对应的Binding实例
func Default(method, contentType string) Binding {
	// Get默认返回Form Binding
	if method == http.MethodGet {
		return Form
	}

	switch contentType {
	case MIMEJSON:
		return JSON
	case MIMEXML, MIMEXML2:
		return XML
	case MIMEPROTOBUF:
		return ProtoBuf
	case MIMEMSGPACK, MIMEMSGPACK2:
		return MsgPack
	case MIMEYAML:
		return YAML
	case MIMETOML:
		return TOML
	case MIMEMultipartPOSTForm:
		return FormMultipart
	default: // case MIMEPOSTForm:
		return Form
	}
}

func validate(obj any) error {
	// Validator为空返回空
	if Validator == nil {
		return nil
	}

	return Validator.ValidateStruct(obj)
}
