package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strings"
)

// 日志级别类型
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

var levelNames = []string{
	"DEBUG",
	"INFO",
	"WARN",
	"ERROR",
	"FATAL",
}

// ANSI 颜色代码
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[37m"
	colorWhite  = "\033[97m"

	// 高亮颜色
	colorRedBold    = "\033[31;1m"
	colorGreenBold  = "\033[32;1m"
	colorYellowBold = "\033[33;1m"
	colorBlueBold   = "\033[34;1m"
	colorPurpleBold = "\033[35;1m"
	colorCyanBold   = "\033[36;1m"
	colorWhiteBold  = "\033[97;1m"
)

// 检查是否为终端
func isTerminal(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		fileInfo, err := f.Stat()
		if err != nil {
			return false
		}
		return (fileInfo.Mode() & os.ModeCharDevice) != 0
	}
	return false
}

// Logger 结构体
type Logger struct {
	*log.Logger
	minLevel     LogLevel
	includeTrace bool
	includePID   bool
	useColor     bool
	output       io.Writer
}

func init() {
	log.SetFlags(log.LstdFlags | log.Llongfile)
}

// 全局默认日志器
var std = NewLogger(os.Stderr, INFO, true)

// 创建新日志实例
func NewLogger(out io.Writer, minLevel LogLevel, includeTrace bool) *Logger {
	return &Logger{
		Logger:       log.New(out, "", log.LstdFlags),
		minLevel:     minLevel,
		includeTrace: includeTrace,
		includePID:   false,
		useColor:     isTerminal(out),
		output:       out,
	}
}

// 设置日志级别
func (l *Logger) SetLevel(level LogLevel) {
	l.minLevel = level
}

// 设置是否包含调用追踪
func (l *Logger) SetIncludeTrace(include bool) {
	l.includeTrace = include
}

// 设置是否包含进程编号
func (l *Logger) SetIncludePID(include bool) {
	l.includePID = include
}

// 设置输出目标
func (l *Logger) SetOutput(w io.Writer) {
	l.output = w
	l.Logger.SetOutput(w)
	l.useColor = isTerminal(w)
}

// 设置时间格式 (空字符串表示不使用时间)
func (l *Logger) SetTimeFormat(format string) {
	if format == "" {
		l.Logger.SetFlags(l.Flags() &^ log.LstdFlags)
	} else {
		l.Logger.SetFlags(l.Flags() | log.LstdFlags)
	}
}

// 启用或禁用颜色输出
func (l *Logger) SetColor(enable bool) {
	l.useColor = enable
}

// 获取调用追踪信息
func (l *Logger) trace(skip int) string {
	if !l.includeTrace {
		return ""
	}

	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return " [???:?]"
	}

	return fmt.Sprintf(" [%s:%d]", file, line)
}

// 获取带颜色的日志级别标签
func (l *Logger) coloredLevel(level LogLevel) string {
	levelName := levelNames[level]

	if !l.useColor {
		return levelName
	}

	switch level {
	case DEBUG:
		return colorCyan + levelName + colorReset
	case INFO:
		return colorGreen + levelName + colorReset
	case WARN:
		return colorYellow + levelName + colorReset
	case ERROR:
		return colorRed + levelName + colorReset
	case FATAL:
		return colorRedBold + levelName + colorReset
	default:
		return levelName
	}
}

// 通用日志输出方法
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	if level < l.minLevel {
		return
	}
	msg := fmt.Sprintf(format, args...)
	if len(args) == 0 || !strings.Contains(format, "%") {
		msg = format + fmt.Sprint(args...)
	}

	trace := l.trace(4)
	coloredLevel := l.coloredLevel(level)

	pid := ""
	if l.includePID {
		pid = fmt.Sprintf(" [PID:%d]", os.Getpid())
	}

	l.Logger.Printf("[%s]%s%s %s", coloredLevel, pid, trace, msg)
}

// 各级别日志方法
func (l *Logger) Debug(format string, v ...interface{}) {
	l.log(DEBUG, format, v...)
}

func (l *Logger) Info(format string, v ...interface{}) {
	l.log(INFO, format, v...)
}

func (l *Logger) Warn(format string, v ...interface{}) {
	l.log(WARN, format, v...)
}

func (l *Logger) Error(format string, v ...interface{}) {
	l.log(ERROR, format, v...)
}

func (l *Logger) Fatal(format string, v ...interface{}) {
	l.log(FATAL, format, v...)
	os.Exit(1)
}

// ================ 全局函数 ================
func SetLevel(level LogLevel)      { std.SetLevel(level) }
func SetOutput(w io.Writer)        { std.SetOutput(w) }
func SetTimeFormat(format string)  { std.SetTimeFormat(format) }
func SetIncludeTrace(include bool) { std.SetIncludeTrace(include) }
func SetIncludePID(include bool)   { std.SetIncludePID(include) }
func SetColor(enable bool)         { std.SetColor(enable) }

func Debug(format string, v ...interface{}) { std.Debug(format, v...) }
func Info(format string, v ...interface{})  { std.Info(format, v...) }
func Warn(format string, v ...interface{})  { std.Warn(format, v...) }
func Error(format string, v ...interface{}) { std.Error(format, v...) }
func Fatal(format string, v ...interface{}) { std.Fatal(format, v...) }
