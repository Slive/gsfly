/*
 * 基于TCP协议的，如TCP，Http和Websocket 的服务监听
 * Author:slive
 * DATE:2020/7/17
 */
package bootstrap

import (
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/xtaci/kcp-go"
	gch "gsfly/channel"
	httpx "gsfly/channel/tcpx/httpx"
	udpx "gsfly/channel/udpx"
	kcpx "gsfly/channel/udpx/kcpx"
	logx "gsfly/logger"
	"net"
	http "net/http"
	"time"
)

var upgrader = websocket.Upgrader{
	HandshakeTimeout: time.Second * 15,
	ReadBufferSize:   10 * 1024,
	WriteBufferSize:  10 * 1024,
}

type HttpWsServerStrap struct {
	ServerStrap
	ServerConf   *HttpxServerConf
	msgHandlers  map[string]*gch.ChannelHandle
	httpHandlers map[string]HttpHandleFunc
}

// Http和Websocket 的服务监听
func NewHttpxServer(parent interface{}, serverConf *HttpxServerConf) IServerStrap {
	t := &HttpWsServerStrap{
		httpHandlers: make(map[string]HttpHandleFunc),
		msgHandlers:  make(map[string]*gch.ChannelHandle),
	}
	t.ServerStrap = *NewServerStrap(parent, serverConf, nil)
	t.ServerConf = serverConf
	return t
}

// AddHttpHandleFunc 添加http处理方法
func (t *HttpWsServerStrap) AddHttpHandleFunc(pattern string, httpHandleFunc HttpHandleFunc) {
	t.httpHandlers[pattern] = httpHandleFunc
}

// AddWsHandleFunc 添加Websocket处理方法
func (t *HttpWsServerStrap) AddWsHandleFunc(pattern string, wsHandleFunc *gch.ChannelHandle) {
	t.msgHandlers[pattern] = wsHandleFunc
}

// GetChHandle 暂时不支持，用GetMsgHandlers()代替
func (t *HttpWsServerStrap) GetChannelHandle() *gch.ChannelHandle {
	logx.Panic("unsupport")
	return nil
}

func (t *HttpWsServerStrap) GetMsgHandlers() map[string]*gch.ChannelHandle {
	return t.msgHandlers
}

func (t *HttpWsServerStrap) Start() error {
	if !t.Closed {
		return errors.New("server had opened, id:" + t.GetId())
	}

	// http处理事件
	httpHandlers := t.httpHandlers
	if httpHandlers != nil {
		for key, f := range httpHandlers {
			http.HandleFunc(key, f)
		}
	}

	defer func() {
		ret := recover()
		if ret != nil {
			logx.Warnf("finish httpws serverstrap, id:%v, ret:%v", t.GetId(), ret)
			t.Stop()
		} else {
			logx.Info("finish httpws serverstrap, id:", t.GetId())
		}
	}()

	wsHandlers := t.GetMsgHandlers()
	if wsHandlers != nil {
		// ws处理事件
		acceptChannels := t.Channels
		for key, f := range wsHandlers {
			http.HandleFunc(key, func(writer http.ResponseWriter, r *http.Request) {
				logx.Info("requestWs:", r.URL)
				err := t.startWs(writer, r, upgrader, t.ServerConf, f, acceptChannels)
				if err != nil {
					logx.Error("start ws error:", err)
				}
			})
		}
	}

	addr := t.ServerConf.GetAddrStr()
	logx.Info("start http listen, addr:", addr)
	err := http.ListenAndServe(addr, nil)
	if err == nil {
		t.Closed = false
	}
	return err
}

type HttpHandleFunc func(http.ResponseWriter, *http.Request)

// startWs 启动ws处理
func (t *HttpWsServerStrap) startWs(w http.ResponseWriter, r *http.Request, upgrader websocket.Upgrader, serverConf *HttpxServerConf, handle *gch.ChannelHandle, acceptChannels map[string]gch.IChannel) error {
	connLen := len(acceptChannels)
	maxAcceptSize := serverConf.GetMaxChannelSize()
	if connLen >= maxAcceptSize {
		return errors.New("max accept size:" + fmt.Sprintf("%v", maxAcceptSize))
	}

	// upgrade处理
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logx.Println("upgrade error:", err)
		return err
	}
	if err != nil {
		logx.Error("accept error:", nil)
		return err
	}

	// OnStopHandle重新b包装
	wsCh := httpx.NewWsChannel(t, conn, serverConf, handle)
	err = wsCh.Start()
	if err == nil {
		// TODO 线程安全？
		acceptChannels[wsCh.GetId()] = wsCh
	}
	return err
}

type KcpServerStrap struct {
	ServerStrap
	ServerConf *KcpServerConf
}

func NewKcpServer(parent interface{}, kcpServerConf *KcpServerConf, chHandle *gch.ChannelHandle) IServerStrap {
	k := &KcpServerStrap{
		ServerConf: kcpServerConf,
	}
	k.ServerStrap = *NewServerStrap(parent, kcpServerConf, chHandle)
	k.ServerConf = kcpServerConf
	return k
}

func (k *KcpServerStrap) Start() error {
	if !k.Closed {
		return errors.New("server had opened, id:" + k.GetId())
	}

	kcpServerConf := k.ServerConf
	addr := kcpServerConf.GetAddrStr()
	logx.Info("listen kcp addr:", addr)
	list, err := kcp.ListenWithOptions(addr, nil, 0, 0)
	if err != nil {
		logx.Info("listen kcp error, addr:", addr, err)
		return err
	}

	defer func() {
		ret := recover()
		if ret != nil {
			logx.Warnf("finish kcp serverstrap, id:%v, ret:%v", k.GetId(), ret)
			k.Stop()
		} else {
			logx.Info("finish kcp serverstrap, id:", k.GetId())
		}
	}()

	kwsChannels := k.Channels
	go func() {
		for {
			kcpConn, err := list.AcceptKCP()
			if err != nil {
				logx.Error("accept kcpconn error:", nil)
				panic(err)
			}
			// OnStopHandle重新包装，以便释放资源
			handle := k.ChannelHandle
			handle.OnStopHandle = ConverOnStopHandle(k.Channels, handle.OnStopHandle)
			kcpCh := kcpx.NewKcpChannel(k, kcpConn, kcpServerConf, handle)
			err = kcpCh.Start()
			if err == nil {
				kwsChannels[kcpCh.GetId()] = kcpCh
			}
		}
	}()

	if err == nil {
		k.Closed = false
	}

	return nil
}

type Kws00ServerStrap struct {
	KcpServerStrap
}

func NewKws00Server(parent interface{}, kcpServerConf *KcpServerConf, onKwsMsgHandle gch.OnMsgHandle,
	onRegisterHandle gch.OnRegisterHandle, onUnRegisterHandle gch.OnUnRegisterHandle) IServerStrap {
	k := &Kws00ServerStrap{}
	k.ServerConf = kcpServerConf
	chHandle := kcpx.NewKws00Handle(onKwsMsgHandle, onRegisterHandle, onUnRegisterHandle)
	k.ServerStrap = *NewServerStrap(parent, kcpServerConf, chHandle)
	return k
}

func (k *Kws00ServerStrap) Start() error {
	if !k.Closed {
		return errors.New("server had opened, id:" + k.GetId())
	}

	kcpServerConf := k.ServerConf
	addr := kcpServerConf.GetAddrStr()
	logx.Info("listen kws00 addr:", addr)
	list, err := kcp.ListenWithOptions(addr, nil, 0, 0)
	if err != nil {
		logx.Info("listen kws00 error, addr:", addr, err)
		return err
	}

	defer func() {
		ret := recover()
		if ret != nil {
			logx.Warnf("finish kws00 serverstrap, id:%v, ret:%v", k.GetId(), ret)
			k.Stop()
		} else {
			logx.Info("finish kws00 serverstrap, id:", k.GetId())
		}
	}()

	kwsChannels := k.Channels
	go func() {
		for {
			kcpConn, err := list.AcceptKCP()
			if err != nil {
				logx.Error("accept kcpconn error:", nil)
				panic(err)
			}

			chHandle := k.ChannelHandle
			kcpCh := kcpx.NewKws00Channel(k, kcpConn, &kcpServerConf.ChannelConf, chHandle)
			kcpCh.ChannelHandle = chHandle
			err = kcpCh.Start()
			if err == nil {
				kwsChannels[kcpCh.GetId()] = kcpCh
			}
		}
	}()
	if err == nil {
		k.Closed = false
	}
	return nil
}

type UdpServerStrap struct {
	ServerStrap
	ServerConf *UdpServerConf
}

func NewUdpServer(parent interface{}, serverConf *UdpServerConf, channelHandle *gch.ChannelHandle) IServerStrap {
	k := &UdpServerStrap{
		ServerConf: serverConf,
	}
	k.ServerStrap = *NewServerStrap(parent, serverConf, channelHandle)
	return k
}

func (u *UdpServerStrap) Start() error {
	serverConf := u.ServerConf
	addr := serverConf.GetAddrStr()
	logx.Info("dial udp addr:", addr)
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		logx.Error("resolve updaddr error:", err)
		return err
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		logx.Error("listen upd error:", err)
		return err
	}

	// TODO udp有源和目标地址之分，待实现
	ch := udpx.NewUdpChannel(u, conn, serverConf, u.ChannelHandle)
	err = ch.Start()
	if err == nil {
		u.Closed = false
	}
	return err
}
