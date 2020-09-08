/*
 * 通信通道，主要是负责通信通道的收发+事件的处理
 * Author:slive
 * DATE:2020/7/17
 */
package channel

import (
	"fmt"
	"github.com/Slive/gsfly/common"
	logx "github.com/Slive/gsfly/logger"
	"github.com/pkg/errors"
	"net"
	"sync"
)

const (
	ERR_READ  = "ERR_READ"
	ERR_MSG   = "ERR_MSG"
	ERR_WRITE = "ERR_WRITE"
	ERR_STOP  = "ERR_STOP"
	ERR_START = "ERR_START"
	ERR_REG   = "ERR_REG"
)

// IChannel 通信通道接口
type IChannel interface {

	// 启动通信通道
	Start() error

	// 停止通道
	Stop()

	// 通过conn写
	WriteByConn(packet IPacket) error

	// 读出错是是否继续
	IsReadLoopContinued(err error) bool

	// NewPacket 创建收发包
	NewPacket() IPacket

	// IsClosed 是否是关闭的
	IsClosed() bool

	// Read 读取方法
	Read() (IPacket, error)

	// Write 写入方法
	Write(packet IPacket) error

	// GetConf 获取通道配置
	GetConf() IChannelConf

	// GetChStatis 获取通道统计相关
	GetChStatis() *ChannelStatis

	LocalAddr() net.Addr

	RemoteAddr() net.Addr

	GetConn() net.Conn

	GetChHandle() *ChannelHandle

	IsRegistered() bool

	SetRegistered(register bool)

	common.IAttact

	common.IParent

	// GetId 通道Id
	common.IId
}

// Channel channel基类
type Channel struct {
	ChannelHandle *ChannelHandle
	ChannelStatis *ChannelStatis
	Conn          net.Conn
	conf          IChannelConf
	readPool      *ReadPool
	closed        bool
	closeExit     chan bool
	registered    bool

	*common.Attact
	// 父接口
	*common.Parent

	*common.Id
}

// 初始化读协程池，全局配置
var def_readPoolConf *ReadPoolConf

var def_readPool *ReadPool

var def_channel_Conf IChannelConf

func initDefChannelConfs() {
	if def_readPool == nil {
		def_readPoolConf = Global_Conf.ReadPoolConf
		def_readPool = NewReadPool(readPoolConf.MaxReadPoolSize, readPoolConf.MaxReadQueueSize)
	}

	if def_channel_Conf == nil {
		def_channel_Conf = Global_Conf.ChannelConf
	}

	logx.Info("init default readPoolConf:", def_readPoolConf)
	logx.Info("init default channelConf:", def_channel_Conf)
}

func InitChannelConfs(rpConf *ReadPoolConf, chConf *ChannelConf) {
	if rpConf == nil {
		err := "ReadPoolConf is nil"
		logx.Panic(err)
		panic(err)
	}
	if chConf == nil {
		err := "ChannelConf is nil"
		logx.Panic(err)
		panic(err)
	}

	def_readPool = NewReadPool(rpConf.MaxReadPoolSize, rpConf.MaxReadQueueSize)
	def_channel_Conf = chConf
}

var initOnce sync.Once

// NewDefChannel 创建默认基础通信通道
func NewDefChannel(parent interface{}, chConf IChannelConf, chHandle *ChannelHandle) *Channel {
	// 全局初始化一次
	return NewChannel(parent, chConf, def_readPool, chHandle)
}

// NewSimpleChannel 创建默认基础通信通道
// msghandle 消息处理
func NewSimpleChannel(msghandle OnMsgHandle) *Channel {
	// 全局初始化一次
	chHandle := NewDefChHandle(msghandle)
	return NewChannel(nil, nil, nil, chHandle)
}

// NewChannel 创建channel
// parent 父节点，可为nil
// chConf channel配置，可为nil，如果为nil，则选用默认
// readPool 读取消息池，可为nil，如果为nil，则选用默认
// chHandle 处理handle，包括读写，注册等处理，不可为空
func NewChannel(parent interface{}, chConf IChannelConf, readPool *ReadPool, chHandle *ChannelHandle) *Channel {
	if chHandle == nil {
		errMsg := "ChannelHandle is nil"
		logx.Error(errMsg)
		panic(errMsg)
	}

	// 如果未初始化一些必要陪着，则默认初始化
	initOnce.Do(func() {
		// 默认初始化logger
		logx.InitDefLogger()
		// 默认初始化channel
		initDefChannelConfs()
	})

	// 选用默认
	if chConf == nil {
		chConf = def_channel_Conf
	}
	if readPool == nil {
		readPool = def_readPool
	}

	channel := &Channel{
		ChannelHandle: chHandle,
		ChannelStatis: NewChStatis(),
		conf:          chConf,
		readPool:      readPool,
		closeExit:     make(chan bool, 1),
	}
	channel.SetClosed(true)
	channel.SetRegistered(false)
	channel.Attact = common.NewAttact()
	channel.Id = common.NewId()
	channel.Parent = common.NewParent(parent)
	logx.Info("create base channel, conf:", chConf)
	return channel
}

func (b *Channel) Start() error {
	return b.StartChannel(b)
}

func (b *Channel) Stop() {
	b.StopChannel(b)
}

func (b *Channel) StartChannel(channel IChannel) error {
	id := b.GetId()
	if !channel.IsClosed() {
		return errors.New("channel is open, chId:" + id)
	}

	handle := channel.GetChHandle()
	defer func() {
		rec := recover()
		if rec != nil {
			logx.Errorf("Start channel error, chId:%v, ret:%v", id, rec)
			err, ok := rec.(error)
			if ok {
				// 捕获处理消息异常
				handle.OnErrorHandle(channel, common.NewError1(ERR_START, err))
			}
			channel.Stop()
		}
	}()
	go b.startReadLoop(channel)

	b.SetClosed(false)

	// 启动后处理方法
	startFunc := handle.OnStartHandle
	if startFunc != nil {
		err := startFunc(channel)
		if err != nil {
			return err
		}
	}

	logx.Info("finish to start channel, chId:", channel.GetId())
	return nil
}

func (b *Channel) NewPacket() IPacket {
	panic("implement me")
}

func (b *Channel) IsClosed() bool {
	return b.closed
}

func (b *Channel) SetClosed(closed bool) {
	b.closed = closed
}

func (b *Channel) Read() (packet IPacket, err error) {
	panic("implement me")
}

func (b *Channel) Write(datapacket IPacket) error {
	if b.IsClosed() {
		return errors.New("wschannel had closed, chId:" + b.GetId())
	}

	channel := datapacket.GetChannel()
	chHandle := channel.GetChHandle()
	defer func() {
		rec := recover()
		if rec != nil {
			logx.Error("write ws error, chId:%v, error:%v", b.GetId(), rec)
			err, ok := rec.(error)
			if !ok {
				err = errors.New(fmt.Sprintf("%v", rec))
			}
			// 捕获处理消息异常
			chHandle.OnErrorHandle(channel, common.NewError1(ERR_WRITE, err))
			// 有异常，终止执行
			channel.Stop()
		}
	}()

	if datapacket.IsPrepare() {
		// 发送前的处理
		befWriteHandle := chHandle.OnBefWriteHandle
		if befWriteHandle != nil {
			err := befWriteHandle(datapacket)
			if err != nil {
				logx.Error("befWriteHandle error:", err)
				return err
			}
		}

		// 发送
		err := channel.WriteByConn(datapacket)
		if err != nil {
			return err
		}

		SendStatis(datapacket, true)
		// 发送成功后的处理
		aftWriteHandle := chHandle.OnAftWriteHandle
		if aftWriteHandle != nil {
			aftWriteHandle(datapacket)
		}
		return err
	} else {
		logx.Warn("datapacket is not prepare.")
	}
	return nil
}

// WriteByConn 实现通过conn发送
func (b *Channel) WriteByConn(datapacket IPacket) error {
	panic("implement me")
}

func (b *Channel) GetConf() IChannelConf {
	return b.conf
}

func (b *Channel) GetChStatis() *ChannelStatis {
	return b.ChannelStatis
}

func (b *Channel) GetConn() net.Conn {
	return b.Conn
}

func (b *Channel) IsReadLoopContinued(err error) bool {
	// 读取超过一定失败次数后，不再继续执行
	return b.GetChStatis().RevStatics.FailTimes < (int64)(b.conf.GetCloseRevFailTime())
}

func (b *Channel) GetChHandle() *ChannelHandle {
	return b.ChannelHandle
}

func (b *Channel) IsRegistered() bool {
	return b.registered
}

func (b *Channel) SetRegistered(register bool) {
	b.registered = register
}

func (b *Channel) StopChannel(channel IChannel) {
	// 关闭状态不再执行后面的内容
	id := b.GetId()
	if channel.IsClosed() {
		logx.Info("channel is closed, chId:", id)
		return
	}

	handle := channel.GetChHandle()
	defer func() {
		rec := recover()
		if rec != nil {
			logx.Warn("close error, chId:", id, rec)
			err, ok := rec.(error)
			if ok {
				// 捕获处理消息异常
				handle.OnErrorHandle(channel, common.NewError1(ERR_STOP, err))
			}
		} else {
			// 执行关闭后的方法
			closeFunc := handle.OnStopHandle
			if closeFunc != nil {
				closeFunc(channel)
			}
			logx.Info("finish to close channel, chId:", id)
		}
	}()

	logx.Info("start to close channel, chId:", id)
	// 清理关闭相关
	b.SetClosed(true)
	b.closeExit <- true
	close(b.closeExit)
	conn := channel.GetConn()
	if conn != nil {
		conn.Close()
	}
}

// StartReadLoop 启动循环读取，读取到数据包后，放入#ReadQueue中，等待处理
func (b *Channel) startReadLoop(channel IChannel) {
	chId := b.GetId()
	defer func() {
		rec := recover()
		if rec != nil {
			logx.Errorf("readloop error, chId:%v, err:%v", chId, rec)
			channel.GetChHandle().OnErrorHandle(channel, common.NewError3(ERR_READ, rec))
			b.Stop()
		}
	}()
	logx.Info("start to readloop, chId:", chId)
	for {
		select {
		case <-b.closeExit:
			logx.Info("stop read loop, chId:", chId)
			return
		default:
			rev, err := channel.Read()
			if err != nil {
				if !channel.IsReadLoopContinued(err) {
					logx.Panic("read loop error:", err)
					panic(err)
				} else {
					continue
				}
			}

			if rev != nil && rev.IsPrepare() {
				readPool := b.readPool
				if readPool != nil {
					// 放入读取协程池等待处理
					readPool.Cache(rev)
				} else {
					// 否则默认直接处理
					channel.GetChHandle().innerMsgHandleFunc(rev)
				}
			}}
	}
}

func (b *Channel) LocalAddr() net.Addr {
	return nil
}

func (b *Channel) RemoteAddr() net.Addr {
	return nil
}
