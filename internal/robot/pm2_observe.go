package robot

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"alemonx/internal/system"
)

var errPM2Offline = errors.New("PM2 守护进程未监听")

// Query the installed daemon directly. The PM2 CLI's jlist will start a
// missing daemon. Axon wraps the JSON RPC call and request ID in an AMP frame.
const pm2MonitorScript = `
const net = require('node:net')
const parts = [
  Buffer.from('j:' + JSON.stringify({ type: 'call', method: 'getMonitorData', args: [{}] })),
  Buffer.from('s:alemonx-monitor')
]
const packet = Buffer.alloc(1 + parts.reduce((size, part) => size + 4 + part.length, 0))
packet[0] = 0x12
let offset = 1
for (const part of parts) {
  packet.writeUInt32BE(part.length, offset)
  offset += 4
  part.copy(packet, offset)
  offset += part.length
}
const socket = net.createConnection(process.argv[1], () => socket.write(packet))
const limit = 16 * 1024 * 1024
const chunks = []
let size = 0
let prefix = Buffer.alloc(0)
let expected = 0
let idOffset = 0
let complete = false
socket.on('data', chunk => {
  size += chunk.length
  if (size > limit) process.exit(3)
  chunks.push(chunk)
  if (expected === 0) {
    prefix = Buffer.concat([prefix, chunk.subarray(0, 5 - prefix.length)])
    if (prefix.length < 5) return
    if (prefix[0] !== 0x12) process.exit(3)
    idOffset = 5 + prefix.readUInt32BE(1)
    expected = idOffset + 4 + parts[1].length
    if (idOffset < 7 || expected > limit) process.exit(3)
  }
  if (size < expected) return
  const reply = Buffer.concat(chunks, size)
  try {
    if (reply.toString('ascii', 5, 7) !== 'j:' ||
        reply.readUInt32BE(idOffset) !== parts[1].length ||
        !reply.subarray(idOffset + 4, expected).equals(parts[1])) process.exit(3)
    const response = JSON.parse(reply.toString('utf8', 7, idOffset))
    if (response.error || !Array.isArray(response.args?.[0])) process.exit(3)
    complete = true
    socket.destroy()
    process.stdout.write(JSON.stringify(response.args[0]))
  } catch {
    process.exit(3)
  }
})
socket.on('error', error => process.exit(['ENOENT', 'ECONNREFUSED'].includes(error.code) ? 2 : 3))
socket.on('end', () => { if (!complete) process.exit(3) })
`

func observePM2Daemon(ctx context.Context, root string) ([]PM2Process, error) {
	ctx, cancel := context.WithTimeout(ctx, pm2ReadTimeout)
	defer cancel()
	runtimePath, err := system.ResolveCommand("node")
	if err != nil {
		runtimePath, err = system.ResolveCommand("bun")
	}
	if err != nil {
		return nil, errors.New("需要 Node.js 或 Bun 才能读取 PM2 守护进程")
	}
	address := pm2SnapshotKey(root)
	if runtime.GOOS == "windows" {
		address = `\\.\pipe\rpc.sock`
	}
	command, err := pm2ReadCommand(ctx, root, runtimePath, "-e", pm2MonitorScript, address)
	if err != nil {
		return nil, err
	}
	env := command.Environ()
	command.Env = []string{}
	// pm2ReadCommand resolves socket aliases for long paths on macOS.
	if runtime.GOOS == "darwin" {
		for _, item := range env {
			if value, ok := strings.CutPrefix(item, "PM2_DAEMON_RPC_PORT="); ok && value != "" {
				if !filepath.IsAbs(value) {
					value = filepath.Join(root, value)
				}
				command.Args[len(command.Args)-1] = value
			}
		}
	}
	for _, item := range env {
		key, _, _ := strings.Cut(item, "=")
		switch strings.ToUpper(key) {
		case "NODE_OPTIONS", "NODE_PATH", "BUN_OPTIONS":
		default:
			command.Env = append(command.Env, item)
		}
	}
	raw, err := command.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 2 {
		return nil, errPM2Offline
	}
	if err != nil {
		return nil, fmt.Errorf("无法读取 PM2 守护进程：%w", err)
	}
	return parsePM2Processes(string(raw))
}
