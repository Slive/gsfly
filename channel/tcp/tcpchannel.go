/*
 * Author:slive
 * DATE:2020/7/17
 */
package tcp

import (
	gch "gsfly/channel"
	"gsfly/config"
	logx "gsfly/logger"
	"net"
	"time"
)

type TcpChannel struct {
	gch.BaseChannel
	conn *net.TCPConn
}

// TODO 配置化
var readbf []byte

func newTcpChannel(tcpConn *net.TCPConn, chConf *config.ChannelConf) *TcpChannel {
	ch := &TcpChannel{conn: tcpConn}
	ch.BaseChannel = *gch.NewDefaultBaseChannel(chConf)
	readBufSize := chConf.ReadBufSize
	if readBufSize <= 0 {
		readBufSize = 10 * 1024
	}
	tcpConn.SetReadBuffer(readBufSize)
	readbf = make([]byte, readBufSize)

	writeBufSize := chConf.WriteBufSize
	if writeBufSize <= 0 {
		writeBufSize = 10 * 1024
	}
	tcpConn.SetWriteBuffer(writeBufSize)
	return ch
}

func NewTcpChannel(tcpConn *net.TCPConn, chConf *config.ChannelConf, msgFunc gch.HandleMsgFunc) *TcpChannel {
	chHandle := gch.NewChHandle(msgFunc, nil, nil)
	return NewTcpChannelWithHandle(tcpConn, chConf, chHandle)
}

func NewTcpChannelWithHandle(tcpConn *net.TCPConn, chConf *config.ChannelConf, chHandle *gch.ChannelHandle) *TcpChannel {
	ch := newTcpChannel(tcpConn, chConf)
	ch.ChannelHandle = *chHandle
	ch.SetChId(tcpConn.LocalAddr().String() + ":" + tcpConn.RemoteAddr().String())
	return ch
}

func (b *TcpChannel) Read() (packet gch.Packet, err error) {
	// TODO 超时配置
	conf := config.Global_Conf.ChannelConf
	b.conn.SetReadDeadline(time.Now().Add(conf.ReadTimeout * time.Second))
	readNum, err := b.conn.Read(readbf)
	if err != nil {
		return nil, err
	}

	bytes := readbf[0:readNum]
	datapack := b.NewPacket()
	datapack.SetData(bytes)
	gch.RevStatis(datapack)
	logx.Info(b.GetChStatis().StringRev())
	return datapack, err
}

func (b *TcpChannel) Write(datapack gch.Packet) error {
	defer func() {
		i := recover()
		if i != nil {
			logx.Error("recover error:", i)
			b.StopChannel(b)
		}
	}()
	if datapack.IsPrepare() {
		bytes := datapack.GetData()
		conf := config.Global_Conf.ChannelConf
		b.conn.SetWriteDeadline(time.Now().Add(conf.WriteTimeout * time.Second))
		_, err := b.conn.Write(bytes)
		if err != nil {
			logx.Error("write error:", err)
			panic(err)
			return nil
		}
		gch.SendStatis(datapack)
		logx.Info(b.GetChStatis().StringSend())
		return err
	}
	return nil
}

func (b *TcpChannel) LocalAddr() net.Addr {
	return b.conn.LocalAddr()
}

func (b *TcpChannel) RemoteAddr() net.Addr {
	return b.conn.RemoteAddr()
}

func (b *TcpChannel) NewPacket() gch.Packet {
	w := &TcpPacket{}
	w.Basepacket = *gch.NewBasePacket(b, gch.PROTOCOL_TCP)
	return w
}

// TcpPacket Tcp包
type TcpPacket struct {
	gch.Basepacket
}
