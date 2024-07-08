// Copyright 2014 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package render

import (
	"html/template"
	"net/http"
)

// 用于HTML模板渲染的左右分割符
type Delims struct {
	// 左分割符（ 默认{{ ）
	Left string
	// 右分割符（ 默认{{ ）
	Right string
}

// HTMLProduction和HTMLDebug的实现接口
type HTMLRender interface {
	// 返回HTML的instance
	Instance(string, any) Render
}

// HTMLProduction包含模板和对应的分割符
type HTMLProduction struct {
	// 模板指针
	Template *template.Template
	// 分隔符
	Delims Delims
}

// HTMLDebug包含模板分隔符、模式和文件列表
type HTMLDebug struct {
	Files   []string
	Glob    string
	Delims  Delims
	FuncMap template.FuncMap
}

// HTML包含模板指针、名字和数据
type HTML struct {
	Template *template.Template
	Name     string
	Data     any
}

// html对应的Content-Type
var htmlContentType = []string{"text/html; charset=utf-8"}

// Instance（HTMLProduction）返回HTML（实现了Render接口）
func (r HTMLProduction) Instance(name string, data any) Render {
	return HTML{
		Template: r.Template,
		Name:     name,
		Data:     data,
	}
}

// Instance (HTMLDebug) 返回HTML（实现了Render接口）
func (r HTMLDebug) Instance(name string, data any) Render {
	return HTML{
		Template: r.loadTemplate(),
		Name:     name,
		Data:     data,
	}
}

// 加载模板
func (r HTMLDebug) loadTemplate() *template.Template {
	// FuncMap初始化
	if r.FuncMap == nil {
		r.FuncMap = template.FuncMap{}
	}
	// 解析文件
	if len(r.Files) > 0 {
		return template.Must(template.New("").Delims(r.Delims.Left, r.Delims.Right).Funcs(r.FuncMap).ParseFiles(r.Files...))
	}
	// TODO：
	if r.Glob != "" {
		return template.Must(template.New("").Delims(r.Delims.Left, r.Delims.Right).Funcs(r.FuncMap).ParseGlob(r.Glob))
	}
	panic("the HTML debug render was created without files or glob pattern")
}

// Render echo HTML数据
func (r HTML) Render(w http.ResponseWriter) error {
	// 写入HTML的Content-Type头
	r.WriteContentType(w)

	if r.Name == "" {
		return r.Template.Execute(w, r.Data)
	}
	return r.Template.ExecuteTemplate(w, r.Name, r.Data)
}

// WriteContentType设置HTML的Content-Type
func (r HTML) WriteContentType(w http.ResponseWriter) {
	writeContentType(w, htmlContentType)
}
