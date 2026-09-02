package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"alemonx/internal/access"
	"alemonx/internal/logging"
	"alemonx/internal/mcp"
	"alemonx/internal/releases"
	"alemonx/internal/resources"
	"alemonx/internal/robot"
	"alemonx/internal/setupplugin"
	"alemonx/internal/system"
	"alemonx/internal/web"
	"alemonx/internal/workspace"
)

//go:embed all:dist
var staticFiles embed.FS

// 前端页面

//go:embed all:resources
var resourceFiles embed.FS

// 开发模板文件 + 机器人启动目录

var Version = "dev"
var FrontendBuild = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "__alx-privileged-run" {
		os.Exit(system.RunPrivilegedHelper(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "__alx-dependency-source" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			os.Exit(2)
		}
		os.Exit(system.DependencySourceOperationHelper(data))
	}
	logging.ConfigureStandardLogger(os.Stderr)
	defer robot.CleanupGitBuildSessions()
	arguments := normalizeArgs(os.Args[1:])
	port, arguments, err := option(arguments, "--port", env("PORT", "17390"))
	if err != nil {
		log.Fatal(err)
	}
	host, arguments, err := option(arguments, "--host", env("alx_BIND", "0.0.0.0"))
	if err != nil {
		log.Fatal(err)
	}
	redisPort, arguments, err := option(arguments, "--redis-port", env("alx_REDIS_PORT", ""))
	if err != nil {
		log.Fatal(err)
	}
	redisOff, arguments := flagPresent(arguments, "--redis-off")
	if strings.TrimSpace(redisPort) != "" {
		value, parseErr := strconv.Atoi(strings.TrimSpace(redisPort))
		if parseErr != nil || value < 1 || value > 65535 {
			log.Fatal("--redis-port 需要在 1-65535 之间")
		}
		redisPort = strconv.Itoa(value)
	}
	mcpPort, arguments, err := option(arguments, "--mcp-port", env("MCP_PORT", "17391"))
	if err != nil {
		log.Fatal(err)
	}
	cwd, arguments, err := option(arguments, "--cwd", ".")
	if err != nil {
		log.Fatal(err)
	}
	workspaceRoot, arguments, err := option(arguments, "--workspace", env("ALX_WORKSPACE", ""))
	if err != nil {
		log.Fatal(err)
	}
	account, arguments, err := option(arguments, "--account", env("alx_AUTH_ACCOUNT", ""))
	if err != nil {
		log.Fatal(err)
	}
	password, arguments, err := option(arguments, "--password", env("alx_AUTH_PASSWORD", ""))
	if err != nil {
		log.Fatal(err)
	}
	confirmation, arguments, err := option(arguments, "--confirm-password", env("alx_AUTH_CONFIRM_PASSWORD", ""))
	if err != nil {
		log.Fatal(err)
	}
	yes, arguments := flagPresent(arguments, "--yes")
	passwordStdin, arguments := flagPresent(arguments, "--password-stdin")
	if len(arguments) > 0 && arguments[0] == "update-watch" {
		if len(arguments) != 1 {
			usage()
			return
		}
		if watchErr := system.WatchUpdate(port); watchErr != nil {
			log.Printf("更新验证失败：%v", watchErr)
		}
		return
	}
	if len(arguments) > 0 && (arguments[0] == "--version" || arguments[0] == "version") {
		fmt.Println(Version)
		return
	}
	if len(arguments) > 0 {
		switch arguments[0] {
		case "mcp-http":
			if len(arguments) != 1 {
				usage()
				return
			}
			token := os.Getenv("MCP_TOKEN")
			if token == "" {
				log.Fatal("请设置 MCP_TOKEN 后再启动 HTTP MCP 服务")
			}
			templates, root := mcpTemplateSource(workspaceRoot)
			serveMCPHTTP(mcpPort, token, mcpPolicy(), templates, root)
			return
		case "mcp":
			if len(arguments) != 1 {
				usage()
				return
			}
			templates, root := mcpTemplateSource(workspaceRoot)
			if err := mcp.NewServerWithPolicyAndWorkspace(Version, templates, mcpPolicy(), root).Serve(os.Stdin, os.Stdout); err != nil {
				log.Printf("MCP 服务已停止：%v", err)
			}
			return
		case "serve":
			if len(arguments) != 1 {
				usage()
				return
			}
		case "install":
			if len(arguments) != 1 {
				usage()
				return
			}
			root, resolveErr := workspace.ResolveRoot(workspaceRoot)
			if resolveErr != nil {
				log.Fatal(resolveErr)
			}
			result, err := system.InstallService(port, host, root)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println(result)
			// Default installs also enable run-without-login on Linux so the
			// service survives reboots and logouts. A permission failure is
			// reported but does not abort the installation.
			if runtime.GOOS == "linux" {
				linger, lingerErr := system.EnableUserLinger()
				if lingerErr != nil {
					fmt.Println(lingerErr.Error() + "（可在 设置 → 服务 中稍后启用无登录运行）")
				} else {
					fmt.Println(linger)
				}
			}
			return
		case "open":
			if len(arguments) != 1 {
				usage()
				return
			}
			if err := system.OpenBrowser(port); err != nil {
				log.Fatal(err)
			}
			fmt.Printf("已打开 http://127.0.0.1:%s\n", port)
			return
		case "update":
			if len(arguments) != 1 {
				usage()
				return
			}
			update, err := releases.SetupUpdate(Version)
			if err != nil {
				log.Fatal(err)
			}
			if !update.Available {
				fmt.Printf("已是最新版本：%s\n", update.Current)
				return
			}
			if !update.PlatformMatched || !update.IntegrityReady {
				fmt.Printf("发现新版本 %s，但没有可安全自动安装的匹配更新包。\n%s\n", update.Latest, update.ReleaseURL)
				return
			}
			result, err := system.ReplaceExecutable(update.DownloadURL, update.AssetName, update.SHA256, update.Latest)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println(result)
			return
		case "status":
			if len(arguments) != 1 {
				usage()
				return
			}
			result, err := system.ServiceStatus()
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println(result)
			return
		case "logs":
			logLines, logArguments, logErr := option(arguments[1:], "--lines", "200")
			if logErr != nil {
				log.Fatal(logErr)
			}
			followLogs, logArguments := flagPresent(logArguments, "--follow")
			if len(logArguments) != 0 {
				usage()
				return
			}
			lines, parseErr := strconv.Atoi(logLines)
			if parseErr != nil || lines <= 0 {
				log.Fatal("--lines 必须是正整数")
			}
			if err := system.StreamServiceLogs(lines, followLogs, os.Stdout); err != nil {
				log.Fatal(err)
			}
			return
		case "health":
			if len(arguments) != 1 {
				usage()
				return
			}
			health, healthErr := system.LocalHealth(port, 3*time.Second)
			if healthErr != nil {
				log.Fatal(healthErr)
			}
			fmt.Println(health)
			return
		case "doctor":
			if len(arguments) != 1 {
				usage()
				return
			}
			printDoctor(port)
			return
		case "start":
			if len(arguments) != 1 {
				usage()
				return
			}
			serviceAction(system.StartService)
			return
		case "stop":
			if len(arguments) != 1 {
				usage()
				return
			}
			serviceAction(system.StopService)
			return
		case "restart":
			if len(arguments) != 1 {
				usage()
				return
			}
			serviceAction(system.RestartService)
			return
		case "uninstall":
			if len(arguments) != 1 || !yes {
				fmt.Println("请使用 alx uninstall --yes 确认移除后台服务。")
				return
			}
			serviceAction(system.UninstallService)
			return
		case "plugin":
			pluginCommand(arguments[1:], yes, workspaceRoot)
			return
		case "auth":
			authCommand(arguments[1:], yes, account, password, confirmation, passwordStdin)
			return
		case "redis":
			redisCommand(arguments[1:], port, account, password)
			return
		case "npm":
			if len(arguments) != 2 || arguments[1] != "publish" {
				usage()
				return
			}
			publish(cwd, "npm-publish", false)
			return
		case "git":
			if len(arguments) != 2 || arguments[1] != "publish" || !yes {
				fmt.Println("请使用 alx --cwd /项目目录 git publish --yes 确认发布。")
				return
			}
			publish(cwd, "git-release", true)
			return
		default:
			usage()
			return
		}
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("ALX_DEPLOYMENT")), "production") {
		if manager, authErr := access.New(); authErr != nil {
			log.Fatalf("生产模式无法加载身份认证配置：%v", authErr)
		} else if status, statusErr := manager.Status(""); statusErr != nil {
			log.Fatalf("生产模式无法读取身份认证状态：%v", statusErr)
		} else if !status.Enabled {
			fmt.Println("提示：生产模式尚未启用本地身份认证，工作台将无认证开放。")
			fmt.Println("请执行 alx auth enable，或启动后在引导页创建管理员账户。")
		}
	}
	if err := system.ConfigurePrivilegedMode(host, strings.EqualFold(strings.TrimSpace(os.Getenv("ALX_DEPLOYMENT")), "production")); err != nil {
		log.Fatal(err)
	}
	serve(host, port, redisPort, redisOff, workspaceRoot)
}

func authCommand(arguments []string, confirmed bool, account, password, confirmation string, passwordStdin bool) {
	manager, err := access.New()
	if err != nil {
		log.Fatal(err)
	}
	if len(arguments) == 1 && arguments[0] == "status" {
		status, err := manager.Status("")
		if err != nil {
			log.Fatal(err)
		}
		if status.Enabled {
			fmt.Printf("身份认证已开启，账户：%s\n配置：%s\n", status.Account, manager.Path())
		} else {
			fmt.Printf("身份认证未开启。\n配置：%s\n", manager.Path())
		}
		return
	}
	if len(arguments) == 1 && arguments[0] == "enable" {
		password, confirmation = authPasswordInput(password, confirmation, passwordStdin)
		if _, err := manager.Enable(account, password, confirmation); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("身份认证已开启，账户：%s\n", strings.TrimSpace(account))
		return
	}
	if len(arguments) == 1 && arguments[0] == "disable" {
		if !confirmed {
			fmt.Println("请使用 alx auth disable --yes 确认关闭身份认证。 ")
			return
		}
		if err := manager.Disable(); err != nil {
			log.Fatal(err)
		}
		fmt.Println("身份认证已关闭。")
		return
	}
	if len(arguments) == 1 && arguments[0] == "reset-super-admin" {
		if !confirmed {
			fmt.Println("此操作会使全部现有登录会话失效，并禁用其他超级管理员。请使用 --yes 确认。")
			return
		}
		password, confirmation = authPasswordInput(password, confirmation, passwordStdin)
		if _, err := manager.ResetSuperAdmin(account, password, confirmation); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("超级管理员已重设为：%s；旧会话已全部失效。\n", strings.TrimSpace(account))
		return
	}
	usage()
}

// authPasswordInput accepts two newline-delimited secrets on stdin so an
// emergency password never has to appear in shell history or process args.
func authPasswordInput(password, confirmation string, passwordStdin bool) (string, string) {
	if !passwordStdin {
		return password, confirmation
	}
	if password != "" || confirmation != "" {
		log.Fatal("--password-stdin 不能与 --password 或 --confirm-password 同时使用")
	}
	reader := bufio.NewReader(io.LimitReader(os.Stdin, 16<<10))
	readSecret := func() string {
		value, err := reader.ReadString('\n')
		if err != nil && len(value) == 0 {
			log.Fatal("无法从标准输入读取密码")
		}
		return strings.TrimSuffix(strings.TrimSuffix(value, "\n"), "\r")
	}
	return readSecret(), readSecret()
}

// redisCommand delegates lifecycle control to the running workbench. The
// server owns the Redis manager, so this command never guesses whether an
// arbitrary process bound to port 6379 is safe to stop.
func redisCommand(arguments []string, workbenchPort, account, password string) {
	if len(arguments) != 1 {
		usage()
		return
	}
	action := arguments[0]
	if action != "status" && action != "start" && action != "stop" && action != "restart" && action != "retry-runtime" {
		usage()
		return
	}
	baseURL := "http://127.0.0.1:" + workbenchPort + "/api/v1/system/redis"
	client := &http.Client{Timeout: 8 * time.Second}
	token := strings.TrimSpace(os.Getenv("ALX_AUTH_TOKEN"))
	if token == "" && strings.TrimSpace(account) != "" && password != "" {
		loginBody, _ := json.Marshal(map[string]string{"account": account, "password": password})
		loginRequest, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+workbenchPort+"/api/v1/auth/login", strings.NewReader(string(loginBody)))
		if err != nil {
			log.Fatal(err)
		}
		loginRequest.Header.Set("Content-Type", "application/json")
		loginResponse, err := client.Do(loginRequest)
		if err != nil {
			log.Fatalf("无法连接正在运行的 ALemonX 服务：%v", err)
		}
		var login struct {
			Token string `json:"token"`
		}
		if loginResponse.StatusCode == http.StatusOK {
			_ = json.NewDecoder(io.LimitReader(loginResponse.Body, 1<<20)).Decode(&login)
			token = login.Token
		}
		_ = loginResponse.Body.Close()
	}
	method := http.MethodGet
	var body io.Reader
	if action != "status" {
		method = http.MethodPost
		body = strings.NewReader(`{"action":"` + action + `"}`)
	}
	request, err := http.NewRequest(method, baseURL, body)
	if err != nil {
		log.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		log.Fatalf("无法连接正在运行的 ALemonX 服务：%v", err)
	}
	defer response.Body.Close()
	var result struct {
		Running  bool   `json:"running"`
		Managed  bool   `json:"managed"`
		External bool   `json:"external"`
		Address  string `json:"address"`
		Message  string `json:"message"`
		Error    string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		log.Fatalf("Redis 控制响应无效：%v", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		if response.StatusCode == http.StatusUnauthorized {
			log.Fatal("Redis 控制需要登录：设置 ALX_AUTH_TOKEN，或使用 --account 和 --password。")
		}
		if result.Error != "" {
			log.Fatal(result.Error)
		}
		log.Fatalf("Redis 控制失败：%s", response.Status)
	}
	state := "已停止"
	if result.Running {
		if result.Managed {
			state = "运行中（ALemonX 管理）"
		} else if result.External {
			state = "运行中（外部 Redis，仅复用）"
		} else {
			state = "运行中"
		}
	}
	fmt.Printf("Redis：%s\n地址：%s\n%s\n", state, result.Address, result.Message)
}

func serve(host, port, redisPort string, redisOff bool, workspaceRoot string) {
	options := web.ServerOptions{RedisDisabled: redisOff, WorkspaceRoot: workspaceRoot, FrontendBuild: FrontendBuild}
	if strings.TrimSpace(redisPort) != "" {
		if value, err := strconv.Atoi(strings.TrimSpace(redisPort)); err == nil {
			options.RedisPort = value
		}
	}
	runtime := web.NewServerRuntimeWithOptions(Version, staticFiles, options, resourceFiles)
	listener, err := net.Listen("tcp", host+":"+port)
	if err != nil {
		log.Fatal(err)
	}
	brokerListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = listener.Close()
		log.Fatal(err)
	}
	runtime.SetPluginDownloadBrokerEndpoint("http://" + brokerListener.Addr().String())
	brokerServer := &http.Server{Handler: runtime.PluginDownloadBrokerHandler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if brokerErr := brokerServer.Serve(brokerListener); brokerErr != nil && brokerErr != http.ErrServerClosed {
			log.Printf("插件官方下载 Broker 已停止：%v", brokerErr)
		}
	}()
	server := &http.Server{
		Handler:           runtime.Handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	stopCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		select {
		case <-stopCtx.Done():
		case <-runtime.UpdateRequested():
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = runtime.Shutdown(shutdownCtx)
		_ = brokerServer.Shutdown(shutdownCtx)
		_ = server.Shutdown(shutdownCtx)
	}()

	fmt.Print(startupMessage(Version, host, port))
	if !isLoopbackHost(host) {
		fmt.Printf("\n  注意：已监听 %s，局域网或公网可直接访问。\n  强烈建议先执行 alx auth enable 开启身份认证，并配合防火墙限制访问来源。\n", host)
	}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// isLoopbackHost reports whether the bind host only accepts local connections.
func isLoopbackHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "", "127.0.0.1", "::1", "localhost":
		return true
	default:
		return false
	}
}

// startupMessage is deliberately written for someone opening alx for the
// first time. The terminal should answer "what is this" and "what next"
// before it exposes implementation details such as the listener address.
func startupMessage(version, host, port string) string {
	addressHost := host
	if addressHost == "0.0.0.0" || addressHost == "::" {
		addressHost = "127.0.0.1"
	}
	address := "http://" + addressHost + ":" + port
	return fmt.Sprintf(`

  ALemonX %s  工作台
  ───────────────────────────────────────

  现在开始
  1. 在浏览器打开：%s
  2. 选择「管理」→ 添加已有机器人目录
  3. 或选择「开发」→ 创建一个新的机器人项目

  接下来你还可以
  · 在「环境」检查 Node.js、Git 与包管理器
  · 在「运行」启动开发、前台或持续运行模式
  · 在「发布」打包并发布到 npm 或 Git Release
  
`, version, address)
}

func serveMCPHTTP(port, token string, policy mcp.Policy, templates fs.FS, workspaceRoot string) {
	server := &http.Server{
		Addr:              "127.0.0.1:" + port,
		Handler:           mcp.NewServerWithPolicyAndWorkspace(Version, templates, policy, workspaceRoot).HTTPHandler(token),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("AlemonJS MCP HTTP 已启动：http://127.0.0.1:%s/mcp", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// mcpTemplateSource materializes the bundled templates into the workspace and
// returns the disk-backed template FS plus the workspace root. When the
// workspace is unavailable the embedded read-only templates are used and the
// workspace root is left empty so the MCP server resolves its own default.
func mcpTemplateSource(workspaceRoot string) (fs.FS, string) {
	templates, err := fs.Sub(resourceFiles, "resources/templates")
	if err != nil {
		return resourceFiles, workspaceRoot
	}
	resourcesRoot, resourcesErr := fs.Sub(resourceFiles, "resources")
	if resourcesErr != nil {
		log.Printf("无法读取嵌入资源根目录：%v", resourcesErr)
		return templates, ""
	}
	layout, workspaceErr := workspace.Ensure(workspaceRoot, templates)
	if workspaceErr != nil {
		log.Printf("工作区初始化失败，MCP 使用内嵌模板：%v", workspaceErr)
		return templates, ""
	}
	resources.Init(resourcesRoot, layout)
	return os.DirFS(layout.Templates()), layout.Root
}

func mcpPolicy() mcp.Policy {
	value := strings.TrimSpace(os.Getenv("MCP_ALLOWED_ROOTS"))
	if value == "" {
		return mcp.Policy{}
	}
	roots := make([]string, 0)
	for _, root := range strings.Split(value, string(os.PathListSeparator)) {
		if root = strings.TrimSpace(root); root != "" {
			roots = append(roots, root)
		}
	}
	return mcp.Policy{AllowedRoots: roots}
}

func publish(root, action string, confirmed bool) {
	result, err := (robot.Manager{}).Run(root, action, "", "", "", "latest", "", confirmed)
	if result.Output != "" {
		fmt.Println(result.Output)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func pluginCommand(arguments []string, confirmed bool, workspaceRoot string) {
	registry := setupplugin.NewWorkspaceRegistry(workspaceRoot)
	if len(arguments) == 1 && arguments[0] == "list" {
		items := registry.All()
		if len(items) == 0 {
			fmt.Println("暂未发现 Setup 插件。")
			return
		}
		for _, plugin := range items {
			state := "已启用"
			if !plugin.Enabled {
				state = "已卸载"
			}
			fmt.Printf("%s\tv%s\t%s\n", plugin.ID, plugin.Version, state)
		}
		return
	}
	if len(arguments) == 2 && arguments[0] == "enable" {
		if err := registry.SetEnabled(arguments[1], true); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("已启用 Setup 插件：%s\n", arguments[1])
		return
	}
	if len(arguments) == 2 && arguments[0] == "install" {
		fmt.Printf("请先查看版本：alx plugin versions %s\n", arguments[1])
		return
	}
	if len(arguments) == 4 && arguments[0] == "install" {
		plugin, err := registry.Install(arguments[1], arguments[2], arguments[3])
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("已安装 Setup 插件：%s v%s\n", plugin.ID, plugin.Version)
		return
	}
	if len(arguments) == 2 && arguments[0] == "disable" {
		if !confirmed {
			fmt.Printf("请使用 alx plugin disable %s --yes 确认卸载。\n", arguments[1])
			return
		}
		if err := registry.SetEnabled(arguments[1], false); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("已卸载 Setup 插件：%s；可用 alx plugin enable %s 恢复。\n", arguments[1], arguments[1])
		return
	}
	if len(arguments) == 2 && arguments[0] == "versions" {
		items, err := registry.Releases(arguments[1])
		if err != nil {
			log.Fatal(err)
		}
		for _, release := range items {
			fmt.Printf("%s\t%s\n", release.Tag, release.Name)
			for _, asset := range release.Assets {
				fmt.Printf("  %s\n", asset.Name)
			}
		}
		return
	}
	usage()
}

func serviceAction(action func() (string, error)) {
	result, err := action()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result)
}

func printDoctor(port string) {
	status, statusErr := system.ServiceStatus()
	if statusErr != nil {
		fmt.Printf("后台服务：无法读取（%v）\n", statusErr)
	} else {
		fmt.Println("后台服务：" + status)
	}
	if health, healthErr := system.LocalHealth(port, 3*time.Second); healthErr != nil {
		fmt.Println("HTTP 健康：不可用（" + healthErr.Error() + "）")
	} else {
		fmt.Println("HTTP 健康：" + health)
	}
	for _, check := range system.NewChecker().CheckGoal("develop", "").Checks {
		fmt.Printf("%s：%s", check.Name, check.Detail)
		if check.Suggestion != "" {
			fmt.Printf("（%s）", check.Suggestion)
		}
		fmt.Println()
	}
}

func option(arguments []string, name, fallback string) (string, []string, error) {
	value := fallback
	remaining := make([]string, 0, len(arguments))
	for index := 0; index < len(arguments); index++ {
		if arguments[index] != name {
			remaining = append(remaining, arguments[index])
			continue
		}
		if index+1 >= len(arguments) || strings.HasPrefix(arguments[index+1], "--") {
			return "", nil, fmt.Errorf("%s 需要一个值", name)
		}
		value = arguments[index+1]
		index++
	}
	return value, remaining, nil
}

func normalizeArgs(arguments []string) []string {
	result := make([]string, len(arguments))
	for index, argument := range arguments {
		result[index] = strings.ReplaceAll(argument, "—", "--")
	}
	return result
}

func flagPresent(arguments []string, name string) (bool, []string) {
	remaining := make([]string, 0, len(arguments))
	present := false
	for _, argument := range arguments {
		if argument == name {
			present = true
			continue
		}
		remaining = append(remaining, argument)
	}
	return present, remaining
}

func usage() {
	fmt.Println(`用法:
  alx [serve] --port 17390           启动浏览器引导（默认监听 0.0.0.0，请先 alx auth enable）
      --host 127.0.0.1               仅本机可访问
      --host 0.0.0.0                 监听所有网卡（默认，局域网/公网可直接访问）
      --workspace <目录>             指定统一工作区（模板、机器人和系统插件；默认 <运行目录>/workspace 或 ALEMONJS_SETUP_ROOTS）
      --redis-port <端口>             调整内置 Redis 端口（默认 6379，会持久化到配置）
      --redis-off                     禁止启动内置 Redis

  alx redis status|start|stop|restart  控制运行中工作台管理的 Redis（认证开启时使用 ALX_AUTH_TOKEN，或 --account/--password）

  alx mcp                            启动本机 stdio MCP 服务
  MCP_TOKEN=... alx mcp-http         启动受保护的本机 HTTP MCP 服务
  alx install --port 17390 [--host 0.0.0.0] [--workspace <目录>]   注册当前程序为后台常驻服务（默认 0.0.0.0；工作区以安装时为准）
  alx open [--port 17390]            打开浏览器
  alx update                         检查并更新 alx
  alx status                         查看后台服务状态
  alx health [--port 17390]          检查本机 AlemonX HTTP 健康状态
  alx doctor [--port 17390]          汇总服务、健康和开发环境诊断
  alx logs [--lines 200] [--follow]  查看后台服务日志（Ctrl+C 结束跟随）
  alx start | stop | restart         管理后台服务
  alx uninstall --yes                移除后台服务
  alx plugin list                     查看已发现的 Setup 插件
  alx plugin versions <id>            查看 Setup 插件 Release 版本与安装包
  alx plugin install <id> <version> <asset>  下载并安装指定 Release 安装包
  alx plugin disable <id> --yes       卸载（停用）一个 Setup 插件
  alx plugin enable <id>              重新启用一个 Setup 插件

  alx auth status                      查看身份认证状态
  alx auth enable --account <账户> --password <密码> --confirm-password <确认密码>
                                     开启本机身份认证（也支持 alx_AUTH_* 环境变量注入）
  alx auth enable --account <账户> --password-stdin
                                     从标准输入读取两次密码，避免出现在命令行
  alx auth disable --yes               关闭身份认证
  alx auth reset-super-admin --account <账户> --password <密码> --confirm-password <确认密码> --yes
                                     紧急重设超级管理员；使旧会话失效并禁用其他超级管理员
  alx auth reset-super-admin --account <账户> --password-stdin --yes
                                     从标准输入读取两次密码，避免出现在命令行
  alx [--cwd /项目目录] npm publish  发布到 npm 官方仓库
  alx [--cwd /项目目录] git publish --yes  创建 GitHub Release 标签`)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
