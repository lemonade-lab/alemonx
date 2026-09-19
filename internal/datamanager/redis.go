package datamanager

import (
	"alemonx/internal/systemnetwork"
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type RedisRequest struct {
	Address   string   `json:"address"`
	Username  string   `json:"username"`
	Password  string   `json:"password"`
	DB        int      `json:"db"`
	Args      []string `json:"args"`
	Confirmed bool     `json:"confirmed"`
	Expected  *string  `json:"expected,omitempty"`
}

var ErrRedisConflict = errors.New("Redis 值已被其他客户端修改或已过期，请重新读取后再保存")

// No scripts, bulk flushes, server configuration, replication or blocking commands.
var redisRead = strings.Fields("PING DBSIZE SCAN TYPE TTL PTTL GET STRLEN HSCAN HGET HLEN LRANGE LLEN SSCAN SCARD ZSCAN ZCARD ZRANGE XRANGE XLEN")
var redisWrite = strings.Fields("SET DEL UNLINK EXPIRE PERSIST RENAME HSET HDEL LPUSH RPUSH LSET LREM LTRIM SADD SREM ZADD ZREM XDEL")

func RedisCommand(ctx context.Context, input RedisRequest) (any, error) {
	if len(input.Args) == 0 || len(input.Args) > 128 || input.DB < 0 || input.DB > 65535 {
		return nil, errors.New("Redis 命令或数据库编号无效")
	}
	command := strings.ToUpper(input.Args[0])
	if input.Expected != nil && (command != "SET" || len(input.Args) != 4 || strings.ToUpper(input.Args[3]) != "KEEPTTL") {
		return nil, errors.New("原值校验仅支持保存字符串并保留 TTL")
	}
	contains := func(items []string) bool {
		for _, item := range items {
			if command == item {
				return true
			}
		}
		return false
	}
	if !contains(redisRead) && !contains(redisWrite) {
		return nil, errors.New("不支持此命令：禁止清库、脚本和服务配置操作")
	}
	if contains(redisWrite) && !input.Confirmed {
		return nil, errors.New("修改 Redis 数据前请确认操作")
	}
	if _, _, err := net.SplitHostPort(input.Address); err != nil {
		return nil, errors.New("连接地址应为 host:port")
	}
	connection, err := (systemnetwork.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", input.Address)
	if err != nil {
		return nil, errors.New("无法连接 Redis，请检查地址和服务状态")
	}
	defer connection.Close()
	deadline := time.Now().Add(5 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = connection.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stop()
	reader := bufio.NewReader(io.LimitReader(connection, 2<<20))
	run := func(args []string) (any, error) {
		var b strings.Builder
		fmt.Fprintf(&b, "*%d\r\n", len(args))
		for _, arg := range args {
			fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(arg), arg)
		}
		if _, err := io.WriteString(connection, b.String()); err != nil {
			return nil, err
		}
		return readRedis(reader, 0)
	}
	if input.Password != "" {
		args := []string{"AUTH", input.Password}
		if input.Username != "" {
			args = []string{"AUTH", input.Username, input.Password}
		}
		if _, err = run(args); err != nil {
			return nil, errors.New("Redis 认证失败")
		}
	}
	if _, err = run([]string{"SELECT", strconv.Itoa(input.DB)}); err != nil {
		return nil, errors.New("无法选择 Redis 数据库")
	}
	if input.Expected != nil {
		if _, err := run([]string{"WATCH", input.Args[1]}); err != nil {
			return nil, err
		}
		current, err := run([]string{"GET", input.Args[1]})
		if err != nil {
			if strings.HasPrefix(err.Error(), "WRONGTYPE") {
				return nil, ErrRedisConflict
			}
			return nil, err
		}
		if current != *input.Expected {
			return nil, ErrRedisConflict
		}
		if _, err := run([]string{"MULTI"}); err != nil {
			return nil, err
		}
		if _, err := run(input.Args); err != nil {
			return nil, err
		}
		committed, err := run([]string{"EXEC"})
		if err != nil {
			return nil, err
		}
		if committed == nil {
			return nil, ErrRedisConflict
		}
		return committed, nil
	}
	return run(input.Args)
}

func readRedis(reader *bufio.Reader, depth int) (any, error) {
	if depth > 8 {
		return nil, errors.New("Redis 响应嵌套过深")
	}
	line, err := reader.ReadString('\n')
	if err != nil || len(line) < 3 {
		return nil, errors.New("Redis 响应不完整或超过 2 MiB")
	}
	value := strings.TrimSuffix(strings.TrimSuffix(line[1:], "\n"), "\r")
	switch line[0] {
	case '+':
		return value, nil
	case '-':
		return nil, errors.New(value)
	case ':':
		return strconv.ParseInt(value, 10, 64)
	case '$', '*':
		n, err := strconv.Atoi(value)
		if err != nil || n < -1 || n > 1<<20 {
			return nil, errors.New("Redis 响应过大或无效")
		}
		if n == -1 {
			return nil, nil
		}
		if line[0] == '$' {
			data := make([]byte, n+2)
			if _, err := io.ReadFull(reader, data); err != nil {
				return nil, err
			}
			if string(data[n:]) != "\r\n" {
				return nil, errors.New("Redis 响应无效")
			}
			if !utf8.Valid(data[:n]) {
				return nil, errors.New("该值包含二进制数据，不能作为 UTF-8 文本编辑")
			}
			return string(data[:n]), nil
		}
		if n > 4096 {
			return nil, errors.New("返回项过多，请使用游标或缩小范围")
		}
		items := make([]any, n)
		for i := range items {
			items[i], err = readRedis(reader, depth+1)
			if err != nil {
				return nil, err
			}
		}
		return items, nil
	}
	return nil, errors.New("不支持的 Redis 响应")
}
