// Copyright 2014 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package gin

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"runtime"
	"strings"
	"time"
)

var (
	dunno     = []byte("???")
	centerDot = []byte("·")
	dot       = []byte(".")
	slash     = []byte("/")
)

// Recovery的函数签名
type RecoveryFunc func(c *Context, err any)

// 返回一个middleware，出现panic时，recovery并回显status code：500
func Recovery() HandlerFunc {
	return RecoveryWithWriter(DefaultErrorWriter)
}

// 返回一个middleware，出现panic时，使用RecoveryFunc进行recovery并回显status code：500
func CustomRecovery(handle RecoveryFunc) HandlerFunc {
	return RecoveryWithWriter(DefaultErrorWriter, handle)
}

// 返回一个middleware，出现panic时，使用writer进行recovery并回显status code：500
func RecoveryWithWriter(out io.Writer, recovery ...RecoveryFunc) HandlerFunc {
	if len(recovery) > 0 {
		return CustomRecoveryWithWriter(out, recovery[0])
	}
	return CustomRecoveryWithWriter(out, defaultHandleRecovery)
}

// 返回一个middleware，出现panic时，使用writer进行recovery，调用提供的handle func，并回显status code：500
func CustomRecoveryWithWriter(out io.Writer, handle RecoveryFunc) HandlerFunc {
	var logger *log.Logger
	if out != nil {
		logger = log.New(out, "\n\n\x1b[31m", log.LstdFlags)
	}
	return func(c *Context) {
		defer func() {
			if err := recover(); err != nil {
				var brokenPipe bool
				// 检查连接是否断开
				if ne, ok := err.(*net.OpError); ok {
					var se *os.SyscallError
					// 如果是连接Error
					if errors.As(ne, &se) {
						seStr := strings.ToLower(se.Error())
						if strings.Contains(seStr, "broken pipe") ||
							strings.Contains(seStr, "connection reset by peer") {
							brokenPipe = true
						}
					}
				}
				if logger != nil {
					stack := stack(3)
					httpRequest, _ := httputil.DumpRequest(c.Request, false)
					// 分割http header
					headers := strings.Split(string(httpRequest), "\r\n")
					// 校验Authorization header
					for idx, header := range headers {
						current := strings.Split(header, ":")
						if current[0] == "Authorization" {
							headers[idx] = current[0] + ": *"
						}
					}
					// 拼接http header
					headersToStr := strings.Join(headers, "\r\n")
					if brokenPipe { // 如果断开连接
						logger.Printf("%s\n%s%s", err, headersToStr, reset)
					} else if IsDebugging() { // 如果是debug模式
						logger.Printf("[Recovery] %s panic recovered:\n%s\n%s\n%s%s",
							timeFormat(time.Now()), headersToStr, err, stack, reset)
					} else { // 其他情况
						logger.Printf("[Recovery] %s panic recovered:\n%s\n%s%s",
							timeFormat(time.Now()), err, stack, reset)
					}
				}
				if brokenPipe { //　如果连接断开，记录Error，终止后续请求
					c.Error(err.(error))
					c.Abort()
				} else { // 没有断开，则通过RecoveryFunc处理
					handle(c, err)
				}
			}
		}()
		c.Next()
	}
}

// 默认的RecoveryFunc，返回status code：500并终止后续请求
func defaultHandleRecovery(c *Context, _ any) {
	c.AbortWithStatus(http.StatusInternalServerError)
}

// 返回有格式的堆栈帧，跳过skip的帧数
func stack(skip int) []byte {
	// 返回的数据
	buf := new(bytes.Buffer)
	// 循环过程中，记录循环打开的文件
	var lines [][]byte
	var lastFile string
	// 跳过skip的帧数
	for i := skip; ; i++ {
		pc, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}
		// 最少打印的数量，如果没找到对应的资源，则不会显示
		fmt.Fprintf(buf, "%s:%d (0x%x)\n", file, line, pc)
		if file != lastFile {
			// 读取file数据
			data, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			// 分割行
			lines = bytes.Split(data, []byte{'\n'})
			lastFile = file
		}
		fmt.Fprintf(buf, "\t%s: %s\n", function(pc), source(lines, line))
	}
	return buf.Bytes()
}

// 返回第n行space-trimmed的切片
func source(lines [][]byte, n int) []byte {
	// stack trace中，index是从1开始，但是array的index是0开始
	n--
	if n < 0 || n >= len(lines) {
		return dunno
	}
	return bytes.TrimSpace(lines[n])
}

// 返回pc的函数名
func function(pc uintptr) []byte {
	// 获取pc的函数指针
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return dunno
	}
	// 转换为[]byte，返回的name中可能包含的各种字符，eg：runtime/debug.*T·ptrmethod，需要的是*T.ptrmethod
	name := []byte(fn.Name())
	// 先删除末尾的/
	if lastSlash := bytes.LastIndex(name, slash); lastSlash >= 0 {
		name = name[lastSlash+1:]
	}
	// 找到.，并且从.开始截断
	if period := bytes.Index(name, dot); period >= 0 {
		name = name[period+1:]
	}
	// 替换所有的·为.
	name = bytes.ReplaceAll(name, centerDot, dot)
	return name
}

// 返回一个Logger的time formatter
func timeFormat(t time.Time) string {
	return t.Format("2006/01/02 - 15:04:05")
}
