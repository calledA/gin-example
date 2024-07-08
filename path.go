// Copyright 2013 Julien Schmidt. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/julienschmidt/httprouter/blob/master/LICENSE.

package gin

// 返回规范的URL path，消除.和..元素，如果结果为空字符串，返回/
// 规则如下：
// 1、使用单个/替换多个//
// 2、消除每个.路径元素，即当前目录 .
// 3、消除每个路径中的..元素，及父目录
// 4、消除根目录开头的/..替换为/
func cleanPath(p string) string {
	// 最长路径128字符
	const stackBufSize = 128
	// 如果为空字符串，返回/
	if p == "" {
		return "/"
	}

	// 一般分配为128字符，如果需要更大的空间，会自动分配大小
	buf := make([]byte, 0, stackBufSize)

	n := len(p)

	// 从path读取，r为下一个需要处理的byte索引
	r := 1
	// 写入buf，w为下一个需要写入的byte索引
	w := 1

	// 路径必须以/开始
	if p[0] != '/' {
		r = 0
		//　判断n与stackBufSize大小
		if n+1 > stackBufSize {
			// 重新分配buf空间
			buf = make([]byte, n+1)
		} else {
			// 优化buf空间
			buf = buf[:n+1]
		}
		// buf[0]添加/
		buf[0] = '/'
	}

	// 是否以/结尾
	trailing := n > 1 && p[n-1] == '/'

	// 循环处理元素
	for r < n {
		switch {
		case p[r] == '/':
			// 空路径，处理下一位元素
			r++

		case p[r] == '.' && r+1 == n:
			// 末尾为.的情况
			trailing = true
			r++

		case p[r] == '.' && p[r+1] == '/':
			// ./的情况，处理后两位
			r += 2

		case p[r] == '.' && p[r+1] == '.' && (r+2 == n || p[r+2] == '/'):
			// ../的情况，处理后三位
			r += 3

			// 回退w的索引到/的位置
			if w > 1 {
				w--

				if len(buf) == 0 {
					for w > 1 && p[w] != '/' {
						w--
					}
				} else {
					for w > 1 && buf[w] != '/' {
						w--
					}
				}
			}

		default:
			// Real path element.
			// Add slash if needed
			// 索引为0的值为/，可以直接跳过第0位索引
			if w > 1 {
				bufApp(&buf, p, w, '/')
				w++
			}

			// 循环复制元素，出现/跳出循环进行其他情况的处理
			for r < n && p[r] != '/' {
				bufApp(&buf, p, w, p[r])
				w++
				r++
			}
		}
	}

	// Re-append trailing slash
	if trailing && w > 1 {
		bufApp(&buf, p, w, '/')
		w++
	}

	// 如果len(buf) == 0，说明path没有被改变，是满足规范的path，返回原始的path string
	if len(buf) == 0 {
		return p[:w]
	}
	// 返回buf的path string
	return string(buf[:w])
}

// 懒创建buf
func bufApp(buf *[]byte, s string, w int, c byte) {
	// 获取buf的[]byte
	b := *buf
	// 没有发生buf赋值，
	if len(b) == 0 {
		// 当下一个字符与原始字符串中的相同，不必分配缓冲区
		if s[w] == c {
			return
		}

		length := len(s)
		//　判断buf缓冲区与s的缓冲区大小
		if length > cap(b) {
			// 创建s的缓冲区大小
			*buf = make([]byte, length)
		} else {
			// 截取s的缓冲区大小
			*buf = (*buf)[:length]
		}
		// 指针赋值
		b = *buf
		// 复制s到buf
		copy(b, s[:w])
	}
	// 修改b[w]为c
	b[w] = c
}
