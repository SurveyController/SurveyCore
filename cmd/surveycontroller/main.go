package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/SurveyController/SurveyConsole/internal/config"
	"github.com/SurveyController/SurveyConsole/internal/engine"
	surveyio "github.com/SurveyController/SurveyConsole/internal/io"
	"github.com/SurveyController/SurveyConsole/internal/logging"
	"github.com/SurveyController/SurveyConsole/internal/models"
	"github.com/SurveyController/SurveyConsole/internal/network/proxy"
	"github.com/SurveyController/SurveyConsole/internal/providers"
)

// Ensure providers.Registry satisfies engine.ProviderRegistry
var _ engine.ProviderRegistry = (*providers.Registry)(nil)

var version = "0.1.0"

func main() {
	// Subcommands
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		cmdRun(os.Args[2:])
	case "parse":
		cmdParse(os.Args[2:])
	case "config":
		cmdConfig(os.Args[2:])
	case "qr":
		cmdQR(os.Args[2:])
	case "export":
		cmdExport(os.Args[2:])
	case "version":
		fmt.Printf("SurveyController-Go v%s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`SurveyController-Go - 问卷自动化处理工具

用法:
  surveycontroller <命令> [选项]

命令:
  run      - 运行问卷提交任务
  parse    - 解析问卷结构
  config   - 配置管理
  qr       - 解析二维码图片中的问卷链接
  export   - 导出运行报告到 Excel
  version  - 显示版本
  help     - 显示帮助

示例:
  surveycontroller run -config config.json -target 10 -threads 3
  surveycontroller parse -url "https://www.wjx.cn/vm/xxxxx.aspx"
  surveycontroller config -create -url "https://www.wjx.cn/vm/xxxxx.aspx"
  surveycontroller qr -image qrcode.png
  surveycontroller export -config config.json -output report.xlsx`)
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := fs.String("config", "", "配置文件路径 (JSON)")
	urlFlag := fs.String("url", "", "问卷链接")
	targetFlag := fs.Int("target", 0, "目标提交份数")
	threadsFlag := fs.Int("threads", 0, "并发线程数")
	randomIPFlag := fs.Bool("random-ip", false, "启用随机 IP")
	proxySourceFlag := fs.String("proxy-source", "", "代理源 (default/benefit/custom)")
	customProxyFlag := fs.String("custom-proxy", "", "自定义代理 API URL")
	proxyAreaFlag := fs.String("proxy-area", "", "官方随机 IP 地区编码")
	randomIPUserIDFlag := fs.Int("random-ip-user-id", 0, "官方随机 IP 用户 ID")
	randomIPDeviceIDFlag := fs.String("random-ip-device-id", "", "官方随机 IP 设备 ID")
	ipExtractEndpointFlag := fs.String("ip-extract-endpoint", "", "官方随机 IP 提取接口")
	verboseFlag := fs.Bool("verbose", false, "详细日志")
	fs.Parse(args)

	if *verboseFlag {
		logging.SetLevel(logging.LevelDebug)
	}

	// Load or create config
	var cfg *models.RuntimeConfig
	if *configPath != "" {
		var err error
		cfg, err = config.LoadFile(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
			os.Exit(1)
		}
	} else {
		defaultCfg := models.NewDefaultRuntimeConfig()
		cfg = &defaultCfg
		if *urlFlag != "" {
			cfg.URL = *urlFlag
		}
	}

	// Override from flags
	if *urlFlag != "" {
		cfg.URL = *urlFlag
	}
	if *targetFlag > 0 {
		cfg.Target = *targetFlag
	}
	if *threadsFlag > 0 {
		cfg.Threads = *threadsFlag
	}
	if *randomIPFlag {
		cfg.RandomIPEnabled = true
	}
	if *proxySourceFlag != "" {
		cfg.ProxySource = *proxySourceFlag
	}
	if *customProxyFlag != "" {
		cfg.CustomProxyAPI = *customProxyFlag
		cfg.ProxySource = "custom"
	}
	if *proxyAreaFlag != "" {
		area := *proxyAreaFlag
		cfg.ProxyAreaCode = &area
	}

	config.MergeDefaults(cfg)

	if cfg.URL == "" {
		fmt.Fprintln(os.Stderr, "错误: 必须提供问卷链接 (-url)")
		os.Exit(1)
	}

	// Parse survey first
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n收到停止信号，正在停止...")
		cancel()
	}()

	registry := providers.Default()

	// Parse survey
	fmt.Printf("正在解析问卷: %s\n", cfg.URL)
	def, err := engine.NewEngine(registry, nil, nil).ParseSurvey(ctx, cfg.URL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "解析问卷失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("解析成功: %s (%d 题)\n", def.Title, len(def.Questions))

	cfg.SurveyTitle = def.Title
	cfg.SurveyProvider = def.Provider

	// Build execution config
	execCfg, err := config.BuildExecutionConfigWithError(cfg, def.Questions)
	if err != nil {
		fmt.Fprintf(os.Stderr, "准备执行配置失败: %v\n", err)
		os.Exit(1)
	}
	state := models.NewExecutionState()
	state.Config = execCfg

	// Create proxy pool if needed
	var pool *proxy.Pool
	if cfg.RandomIPEnabled {
		areaCode := ""
		if cfg.ProxyAreaCode != nil {
			areaCode = *cfg.ProxyAreaCode
		}
		pool = proxy.NewPool(
			cfg.ProxySource,
			cfg.CustomProxyAPI,
			proxy.WithOfficialAreaCode(areaCode),
			proxy.WithOfficialCredentials(*randomIPUserIDFlag, *randomIPDeviceIDFlag),
			proxy.WithOfficialEndpoint(*ipExtractEndpointFlag),
		)
		fmt.Println("随机 IP 已启用")
	}

	// Status handler
	handler := func(event engine.StatusEvent) {
		if event.Success {
			fmt.Printf("[✓] %s - %s\n", event.ThreadName, event.StatusText)
		} else if event.Fail {
			fmt.Printf("[✗] %s - %s\n", event.ThreadName, event.StatusText)
		}
	}

	// Run
	fmt.Printf("开始执行: 目标 %d 份, 并发 %d 线程\n", cfg.Target, cfg.Threads)
	e := engine.NewEngine(registry, pool, handler)
	if err := e.Run(ctx, execCfg, state); err != nil {
		fmt.Fprintf(os.Stderr, "执行失败: %v\n", err)
		os.Exit(1)
	}

	// Report results
	fmt.Printf("\n执行完成: 成功 %d, 失败 %d\n", state.GetCurNum(), state.GetCurFail())
}

func cmdParse(args []string) {
	fs := flag.NewFlagSet("parse", flag.ExitOnError)
	urlFlag := fs.String("url", "", "问卷链接")
	outputFlag := fs.String("output", "", "输出文件路径 (JSON)")
	fs.Parse(args)

	if *urlFlag == "" {
		fmt.Fprintln(os.Stderr, "错误: 必须提供问卷链接 (-url)")
		os.Exit(1)
	}

	ctx := context.Background()
	registry := providers.Default()
	e := engine.NewEngine(registry, nil, nil)

	fmt.Printf("正在解析: %s\n", *urlFlag)
	def, err := e.ParseSurvey(ctx, *urlFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "解析失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Provider: %s\n", def.Provider)
	fmt.Printf("标题: %s\n", def.Title)
	fmt.Printf("题目数: %d\n\n", len(def.Questions))

	for _, q := range def.Questions {
		fmt.Printf("  第 %d 题 [%s]: %s\n", q.Num, q.TypeCode, q.Title)
		if len(q.OptionTexts) > 0 {
			for i, opt := range q.OptionTexts {
				fmt.Printf("    %d. %s\n", i+1, opt)
			}
		}
	}

	if *outputFlag != "" {
		data, err := json.MarshalIndent(def, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "序列化结果失败: %v\n", err)
			os.Exit(1)
		}
		if err := os.MkdirAll(filepath.Dir(*outputFlag), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "创建输出目录失败: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(*outputFlag, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "保存结果失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\n结果已保存到: %s\n", *outputFlag)
	}
}

func cmdConfig(args []string) {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	createFlag := fs.Bool("create", false, "创建新配置")
	urlFlag := fs.String("url", "", "问卷链接")
	outputFlag := fs.String("output", "config.json", "输出路径")
	fs.Parse(args)

	if *createFlag {
		cfg := models.NewDefaultRuntimeConfig()
		if *urlFlag != "" {
			cfg.URL = *urlFlag
			ctx := context.Background()
			registry := providers.Default()
			e := engine.NewEngine(registry, nil, nil)
			fmt.Printf("正在解析问卷: %s\n", cfg.URL)
			def, err := e.ParseSurvey(ctx, cfg.URL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "解析问卷失败: %v\n", err)
				os.Exit(1)
			}
			cfg.SurveyTitle = def.Title
			cfg.SurveyProvider = def.Provider
			cfg.QuestionsInfo = models.CloneSurveyQuestionMetas(def.Questions)
			cfg.QuestionEntries = config.BuildDefaultQuestionEntries(def.Questions, nil)
			fmt.Printf("解析成功: %s (%d 题)，已生成 %d 条默认题目配置\n", def.Title, len(def.Questions), len(cfg.QuestionEntries))
		}
		if err := config.SaveFile(&cfg, *outputFlag); err != nil {
			fmt.Fprintf(os.Stderr, "保存配置失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("配置已保存到: %s\n", *outputFlag)
	} else {
		fmt.Println("用法: surveycontroller config -create [-url URL] [-output path]")
	}
}

func cmdQR(args []string) {
	fs := flag.NewFlagSet("qr", flag.ExitOnError)
	imageFlag := fs.String("image", "", "二维码图片路径")
	outputFlag := fs.String("output", "", "输出文件路径 (可选)")
	fs.Parse(args)

	if *imageFlag == "" {
		fmt.Fprintln(os.Stderr, "错误: 必须提供二维码图片路径 (-image)")
		os.Exit(1)
	}

	fmt.Printf("正在解析二维码: %s\n", *imageFlag)
	url, err := surveyio.DecodeSurveyURLFromFile(*imageFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "解析失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("解析成功!\n")
	fmt.Printf("问卷链接: %s\n", url)

	if *outputFlag != "" {
		if err := os.WriteFile(*outputFlag, []byte(url), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "保存失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("链接已保存到: %s\n", *outputFlag)
	}
}

func cmdExport(args []string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	configFlag := fs.String("config", "", "配置文件路径")
	outputFlag := fs.String("output", "report.xlsx", "输出 Excel 路径")
	fs.Parse(args)

	if *configFlag == "" {
		fmt.Fprintln(os.Stderr, "错误: 必须提供配置文件路径 (-config)")
		os.Exit(1)
	}

	cfg, err := config.LoadFile(*configFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}

	state := models.NewExecutionState()
	state.Config = config.BuildExecutionConfig(cfg, cfg.QuestionsInfo)

	fmt.Printf("正在导出报告: %s\n", *outputFlag)
	if err := surveyio.ExportRunReport(*outputFlag, cfg, state, cfg.QuestionsInfo); err != nil {
		fmt.Fprintf(os.Stderr, "导出失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("报告已导出: %s\n", *outputFlag)
}
