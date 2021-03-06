/*
 * Author:slive
 * DATE:2020/7/24
 */
package logger

import (
	"fmt"
	"github.com/slive/gsfly/common"
	"github.com/slive/gsfly/util"
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/rifflock/lfshook"
	logx "github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/writer"
	"io/ioutil"
	"log"
	"os"
	"path"
	"runtime"
	"strings"
	"time"
)

const (
	Level_Debug = "debug"
	Level_Info  = "info"
	Level_Warn  = "warn"
	Level_Error = "error"
)

type LogConf struct {
	// LogFile 日志文件
	LogFile string

	// LogDir 日志路径
	LogDir string

	Level string

	MaxRemainCount uint
}

func (logConf *LogConf) GetLevel() logx.Level {
	level := logConf.Level
	if len(level) <= 0 {
		level = Level_Debug
	} else {
		level = strings.ToLower(level)
	}
	switch level {
	case Level_Debug:
		return logx.DebugLevel
	case Level_Info:
		return logx.InfoLevel
	case Level_Warn:
		return logx.WarnLevel
	case Level_Error:
		return logx.ErrorLevel
	default:
		return logx.DebugLevel
	}
}

func newDefaultLogConf() *LogConf {
	logConf := &LogConf{
		LogFile:        "log-gsfly.log",
		LogDir:         defaultLogDir(),
		MaxRemainCount: 20,
		Level:          Level_Debug,
	}
	return logConf
}

func defaultLogDir() string {
	return util.GetPwd() + "/log"
}

var logLevel logx.Level

var initLogConf *LogConf

func InitDefLogger() {
	if initLogConf == nil {
		InitLogger(newDefaultLogConf())
	}
}

func InitLogger(logConf *LogConf) {
	logdir := logConf.LogDir
	// 创建日志文件夹
	if len(logdir) <= 0 {
		logdir = defaultLogDir()
	}
	_ = mkdirLog(logdir)

	filePath := path.Join(logdir, logConf.LogFile)
	log.Println("filePath:", filePath)
	var logfile *os.File = nil
	if util.CheckFileExist(filePath) {
		// 文件存在，打开
		logfile, _ = os.OpenFile(filePath, os.O_APPEND|os.O_RDWR, 0644)
	} else {
		// 文件不存在，创建
		logfile, _ = os.OpenFile(filePath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	}
	logLevel = logConf.GetLevel()
	log.Println("default level:", logLevel)
	logx.SetLevel(logLevel)
	logx.SetOutput(ioutil.Discard) // Send all logs to nowhere by default
	logx.AddHook(&writer.Hook{ // Send logs with level higher than warning to stderr
		Writer: logfile,
		LogLevels: []logx.Level{
			logx.PanicLevel,
			logx.FatalLevel,
			logx.ErrorLevel,
			logx.WarnLevel,
			logx.InfoLevel,
			logx.DebugLevel,
		},
	})

	logx.AddHook(&writer.Hook{ // Send info and debug logs to stdout
		Writer: os.Stdout,
		LogLevels: []logx.Level{
			logx.InfoLevel,
			logx.DebugLevel,
		},
	})

	logx.AddHook(&writer.Hook{ // Send logs with level higher than warning to stderr
		Writer: os.Stderr,
		LogLevels: []logx.Level{
			logx.PanicLevel,
			logx.FatalLevel,
			logx.ErrorLevel,
			logx.WarnLevel,
		},
	})

	tf := logx.TextFormatter{
		DisableTimestamp: false,
		FullTimestamp:    true,
		TimestampFormat:  "2006-01-02 15:04:05.999999999",
		DisableSorting:   false,
	}
	logx.SetFormatter(&tf)
	maxRemainCount := logConf.MaxRemainCount
	if maxRemainCount <= 0 {
		maxRemainCount = 100
	}
	logx.AddHook(newRlfHook(maxRemainCount, filePath, &tf))
	initLogConf = logConf
}

func mkdirLog(dir string) (e error) {
	_, er := os.Stat(dir)
	defer func() {
		err := recover()
		if err != nil {
			log.Println("mkdirLog error:", err)
		}
	}()
	b := er == nil || os.IsExist(er)
	if !b {
		if err := os.MkdirAll(dir, 0775); err != nil {
			if os.IsPermission(err) {
				er = err
			}
		}
	}
	return er
}

func newRlfHook(maxRemainCont uint, logName string, tf *logx.TextFormatter) logx.Hook {
	writer, err := rotatelogs.New(logName+"%Y%m%d",
		rotatelogs.WithLinkName(logName),
		rotatelogs.WithRotationTime(time.Hour*24),
		rotatelogs.WithRotationCount(maxRemainCont))
	if err != nil {
		logx.Errorf("config local file system for logger error: %v", err)
	}

	lfsHook := lfshook.NewHook(lfshook.WriterMap{
		logx.DebugLevel: writer,
		logx.InfoLevel:  writer,
		logx.WarnLevel:  writer,
		logx.ErrorLevel: writer,
		logx.FatalLevel: writer,
		logx.PanicLevel: writer,
	}, tf)

	return lfsHook
}

func caller() string {
	return callerWithSkip(3)
}

func callerWithSkip(skip int) string {
	lfmt := ""
	pc, file, line, ok := runtime.Caller(skip)
	if ok {
		sps := strings.Split(file, "/")
		if sps != nil {
			// 取缩写路径如：logger/logger.go
			flen := len(sps)
			if flen >= 2 {
				file = sps[flen-2] + "/"
			} else {
				file = ""
			}
			file += sps[flen-1]
		}
		lfmt = file

		forPC := runtime.FuncForPC(pc)
		if forPC != nil {
			// 获取文件所属方法
			fnName := forPC.Name()
			fs := strings.Split(fnName, ".")
			if fs != nil {
				fnName = fs[len(fs)-1]
			}
			lfmt += fmt.Sprintf("#%v", fnName)
		}

		// 获取行数
		lfmt += fmt.Sprintf("(%v) ", line)
	}
	return lfmt
}

func Println(logs ...interface{}) {
	field := logx.WithField("file", caller())
	field.Println(logs...)
}

func Printf(format string, logs ...interface{}) {
	field := logx.WithField("file", caller())
	field.Printf(format, logs...)
}

func PrintTracef(ctx common.IRunContext, format string, logs ...interface{}) {
	field := withField(ctx)
	field.Printf(format, logs...)
}

func withField(ctx common.IRunContext) *logx.Entry {
	field := logx.WithField("file", callerWithSkip(3))
	if ctx != nil {
		field = field.WithField("trace", ctx.GetTrace())
	}
	return field
}

func IsDebug() bool {
	return logx.IsLevelEnabled(logx.DebugLevel)
}

func Debug(logs ...interface{}) {
	field := logx.WithField("file", caller())
	field.Debug(logs...)
}

func Debugf(format string, logs ...interface{}) {
	field := logx.WithField("file", caller())
	field.Debugf(format, logs...)
}

func DebugTracef(ctx common.IRunContext, format string, logs ...interface{}) {
	field := withField(ctx)
	field.Debugf(format, logs...)
}

func Info(logs ...interface{}) {
	field := logx.WithField("file", caller())
	field.Info(logs...)
}

func InfoTrace(ctx common.IRunContext, logs ...interface{}) {
	field := withField(ctx)
	field.Info(logs...)
}

func Infof(format string, logs ...interface{}) {
	field := logx.WithField("file", caller())
	field.Infof(format, logs...)
}

func InfoTracef(ctx common.IRunContext, format string, logs ...interface{}) {
	field := withField(ctx)
	field.Infof(format, logs...)
}

func Warn(logs ...interface{}) {
	field := logx.WithField("file", caller())
	field.Warn(logs...)
}

func Warnf(format string, logs ...interface{}) {
	field := logx.WithField("file", caller())
	field.Warnf(format, logs...)
}

func WarnTracef(ctx common.IRunContext, format string, logs ...interface{}) {
	field := withField(ctx)
	field.Warnf(format, logs...)
}

func Error(logs ...interface{}) {
	field := logx.WithField("file", caller())
	field.Error(logs...)
}

func Errorf(format string, logs ...interface{}) {
	field := logx.WithField("file", caller())
	field.Errorf(format, logs...)
}

func ErrorTracef(ctx common.IRunContext, format string, logs ...interface{}) {
	field := withField(ctx)
	field.Errorf(format, logs...)
}

func Fatal(logs ...interface{}) {
	field := logx.WithField("file", caller())
	field.Fatal(logs...)
}

func Fatalf(format string, logs ...interface{}) {
	field := logx.WithField("file", caller())
	field.Fatalf(format, logs...)
}

func FatalTracef(ctx common.IRunContext, format string, logs ...interface{}) {
	field := withField(ctx)
	field.Fatalf(format, logs...)
}

func Panic(logs ...interface{}) {
	field := logx.WithField("file", caller())
	field.Panic(logs...)
	panic(logs)
}

func Panicf(format string, logs ...interface{}) {
	field := logx.WithField("file", caller())
	field.Panicf(format, logs...)
	panic(logs)
}

// PanicTracef 支持
func PanicTracef(ctx common.IRunContext, format string, logs ...interface{}) {
	field := withField(ctx)
	field.Panicf(format, logs...)
	panic(logs)
}
