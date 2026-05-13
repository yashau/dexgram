//go:generate go run genversion.go
//go:generate goversioninfo -64 -o resource.syso

package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"dexgram/internal/config"
	"dexgram/internal/state"

	"github.com/go-telegram/bot"
)

const logFileMaxLines = 5000

func main() {
	exitCode := 0
	defer func() {
		if recovered := recover(); recovered != nil {
			reportFatalPanic(recovered)
			exitCode = 2
		}
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	}()
	if err := run(); err != nil {
		reportFatalError(err)
		exitCode = 1
	}
}

func run() error {
	if len(os.Args) > 1 && os.Args[1] == "service" {
		return runServiceCommand(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] == "onboard" {
		return runOnboardCommand(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] == "update" {
		return runUpdateCommand()
	}
	if len(os.Args) > 1 && os.Args[1] == "version" {
		printVersion(os.Stdout)
		return nil
	}

	var configPath string
	var logPath string
	var checkOnly bool
	var showVersion bool
	fs := flag.NewFlagSet(filepath.Base(os.Args[0]), flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	fs.StringVar(&configPath, "config", defaultConfigPath(), "path to Dexgram TOML config")
	fs.StringVar(&logPath, "log", "", "append daemon logs to this file")
	fs.BoolVar(&checkOnly, "check", false, "validate Telegram setup and exit before polling")
	fs.BoolVar(&showVersion, "version", false, "print Dexgram version and exit")
	fs.Usage = func() {
		printHelp(fs.Output(), filepath.Base(os.Args[0]), fs)
	}
	if err := fs.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if showVersion {
		printVersion(os.Stdout)
		return nil
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unknown command or argument %q; run %s --help", fs.Arg(0), filepath.Base(os.Args[0]))
	}

	closeLog, err := configureLogFile(logPath)
	if err != nil {
		return err
	}
	defer closeLog()

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	store, err := state.Open("")
	if err != nil {
		return err
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("close state store: %v", err)
		}
	}()
	log.Printf("state path: %s", store.Path())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := &app{
		cfg:       cfg,
		store:     store,
		active:    map[string]*activeTurn{},
		approvals: map[string]*pendingApproval{},
		actions:   map[string]turnAction{},
		inputs:    map[string]*pendingInput{},
	}
	if _, err := app.refreshProjects(); err != nil {
		return err
	}
	tg, err := bot.New(cfg.Telegram.BotToken,
		bot.WithDefaultHandler(app.handleUpdateFatal),
		bot.WithErrorsHandler(func(err error) {
			log.Printf("telegram error: %v", err)
		}),
	)
	if err != nil {
		return err
	}
	app.bot = tg

	me, err := tg.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("get bot identity: %w", err)
	}
	log.Printf("connected to Telegram as @%s (%d)", me.Username, me.ID)

	if err := ensureThreadedMode(ctx, tg, me, cfg.Telegram.ChatID); err != nil {
		return err
	}

	if err := reconcileCommands(ctx, tg, cfg.Telegram.ChatID); err != nil {
		return err
	}
	log.Printf("telegram slash commands cleared and registered")
	log.Printf("codex mode approval_policy=%s sandbox=%s", cfg.Codex.ApprovalPolicy, cfg.Codex.Sandbox)

	if checkOnly {
		log.Printf("telegram setup check passed")
		return nil
	}

	if cfg.Telegram.ChatID == 0 {
		log.Printf("telegram.chat_id is 0; logging updates from any chat for discovery")
	} else {
		log.Printf("listening only in private topic chat_id=%d", cfg.Telegram.ChatID)
	}
	tg.Start(ctx)
	return nil
}

func defaultConfigPath() string {
	local := "dexgram.toml"
	if _, err := os.Stat(local); err == nil {
		return local
	}
	return mustServiceConfigPath()
}

func configureLogFile(path string) (func(), error) {
	if path == "" {
		return func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	f, err := openLineLimitedLogFile(path, logFileMaxLines)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", path, err)
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	return func() {
		log.SetOutput(os.Stderr)
		_ = f.Close()
	}, nil
}

type lineLimitedLogFile struct {
	path      string
	maxLines  int
	file      *os.File
	lineCount int
}

func openLineLimitedLogFile(path string, maxLines int) (*lineLimitedLogFile, error) {
	if err := trimLogFileLines(path, maxLines); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	lineCount, err := countLogFileLines(path)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &lineLimitedLogFile{
		path:      path,
		maxLines:  maxLines,
		file:      f,
		lineCount: lineCount,
	}, nil
}

func (f *lineLimitedLogFile) Write(p []byte) (int, error) {
	n, err := f.file.Write(p)
	if n <= 0 || f.maxLines <= 0 {
		return n, err
	}
	f.lineCount += countTextLines(p[:n])
	if f.lineCount <= f.maxLines {
		return n, err
	}
	if trimErr := f.trim(); err == nil {
		err = trimErr
	}
	return n, err
}

func (f *lineLimitedLogFile) Close() error {
	return f.file.Close()
}

func (f *lineLimitedLogFile) trim() error {
	closeErr := f.file.Close()
	trimErr := trimLogFileLines(f.path, f.maxLines)
	reopened, reopenErr := os.OpenFile(f.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if reopenErr == nil {
		f.file = reopened
		if lineCount, err := countLogFileLines(f.path); err == nil {
			f.lineCount = lineCount
		} else {
			f.lineCount = f.maxLines
			reopenErr = err
		}
	}
	if closeErr != nil {
		return closeErr
	}
	if trimErr != nil {
		return trimErr
	}
	return reopenErr
}

func trimLogFileLines(path string, maxLines int) error {
	if maxLines <= 0 {
		return nil
	}
	in, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	reader := bufio.NewReader(in)
	ring := make([]string, maxLines)
	lineCount := 0
	for {
		line, readErr := reader.ReadString('\n')
		if line != "" {
			ring[lineCount%maxLines] = line
			lineCount++
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			_ = in.Close()
			return readErr
		}
	}
	if err := in.Close(); err != nil {
		return err
	}
	if lineCount <= maxLines {
		return nil
	}

	out, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()
	start := lineCount % maxLines
	for i := 0; i < maxLines; i++ {
		if _, err := io.WriteString(out, ring[(start+i)%maxLines]); err != nil {
			return err
		}
	}
	return nil
}

func countLogFileLines(path string) (int, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = f.Close()
	}()
	reader := bufio.NewReader(f)
	lineCount := 0
	for {
		line, readErr := reader.ReadString('\n')
		if line != "" {
			lineCount++
		}
		if errors.Is(readErr, io.EOF) {
			return lineCount, nil
		}
		if readErr != nil {
			return 0, readErr
		}
	}
}

func countTextLines(text []byte) int {
	if len(text) == 0 {
		return 0
	}
	lines := 0
	for _, b := range text {
		if b == '\n' {
			lines++
		}
	}
	if text[len(text)-1] != '\n' {
		lines++
	}
	return lines
}

func reportFatalError(err error) {
	line := fmt.Sprintf("error: %v", err)
	fmt.Fprintln(os.Stderr, line)
	appendFatalLog(fatalLogPathFromArgs(os.Args[1:]), "fatal "+line+"\n")
}

func reportFatalPanic(recovered any) {
	stack := debug.Stack()
	message := fmt.Sprintf("panic: %v", recovered)
	fmt.Fprintln(os.Stderr, message)
	_, _ = os.Stderr.Write(stack)
	appendFatalLog(fatalLogPathFromArgs(os.Args[1:]), fmt.Sprintf("fatal %s\n%s", message, stack))
}

func goFatal(fn func()) {
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				reportFatalPanic(recovered)
				os.Exit(2)
			}
		}()
		fn()
	}()
}

func appendFatalLog(path, message string) {
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error: write fatal log: %v\n", err)
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: write fatal log: %v\n", err)
		return
	}
	timestamp := time.Now().Format("2006/01/02 15:04:05")
	if _, err := fmt.Fprintf(f, "%s %s", timestamp, message); err != nil {
		fmt.Fprintf(os.Stderr, "error: write fatal log: %v\n", err)
	}
	if err := f.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "error: close fatal log: %v\n", err)
		return
	}
	if err := trimLogFileLines(path, logFileMaxLines); err != nil {
		fmt.Fprintf(os.Stderr, "error: trim fatal log: %v\n", err)
	}
}

func fatalLogPathFromArgs(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-log" || arg == "--log" {
			if i+1 < len(args) {
				return args[i+1]
			}
			return ""
		}
		if path, ok := strings.CutPrefix(arg, "-log="); ok {
			return path
		}
		if path, ok := strings.CutPrefix(arg, "--log="); ok {
			return path
		}
	}
	return ""
}
