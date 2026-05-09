package service

import (
	"crypto/rand"
	"errors"
	"net"
	"sync"

	"golang.org/x/crypto/blake2b"
)

// Hysteria2 salamander 混淆：每个 UDP 包形如  salt(8B) || body XOR keystream，
// keystream = BLAKE2b-256(psk || salt) 循环重复。服务端开启 obfs 后，所有未做
// 该变换的 QUIC 包会被静默丢弃，导致探测端永远等不到回包。

const salamanderSaltLen = 8

type salamanderObfs struct {
	psk []byte
}

func newSalamander(password string) *salamanderObfs {
	return &salamanderObfs{psk: []byte(password)}
}

func (o *salamanderObfs) keystream(salt []byte) [blake2b.Size256]byte {
	buf := make([]byte, 0, len(o.psk)+len(salt))
	buf = append(buf, o.psk...)
	buf = append(buf, salt...)
	return blake2b.Sum256(buf)
}

func (o *salamanderObfs) obfuscate(in, out []byte) (int, error) {
	if len(out) < len(in)+salamanderSaltLen {
		return 0, errors.New("salamander: out buffer too small")
	}
	if _, err := rand.Read(out[:salamanderSaltLen]); err != nil {
		return 0, err
	}
	key := o.keystream(out[:salamanderSaltLen])
	for i, b := range in {
		out[salamanderSaltLen+i] = b ^ key[i%len(key)]
	}
	return len(in) + salamanderSaltLen, nil
}

func (o *salamanderObfs) deobfuscate(in, out []byte) (int, error) {
	if len(in) < salamanderSaltLen+1 {
		return 0, errors.New("salamander: input too short")
	}
	bodyLen := len(in) - salamanderSaltLen
	if len(out) < bodyLen {
		return 0, errors.New("salamander: out buffer too small")
	}
	key := o.keystream(in[:salamanderSaltLen])
	for i, b := range in[salamanderSaltLen:] {
		out[i] = b ^ key[i%len(key)]
	}
	return bodyLen, nil
}

const maxObfsPacket = 65535

// obfsPacketConn 包装 net.PacketConn，对每个 UDP 数据报自动应用 salamander。
// 解混淆失败的包会被静默丢弃并继续读下一个，避免无关 UDP 流量打断 quic-go。
type obfsPacketConn struct {
	net.PacketConn
	obfs *salamanderObfs
	pool sync.Pool
}

func newObfsPacketConn(pc net.PacketConn, obfs *salamanderObfs) *obfsPacketConn {
	return &obfsPacketConn{
		PacketConn: pc,
		obfs:       obfs,
		pool: sync.Pool{New: func() interface{} {
			b := make([]byte, maxObfsPacket)
			return &b
		}},
	}
}

func (c *obfsPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	bufp := c.pool.Get().(*[]byte)
	defer c.pool.Put(bufp)
	buf := *bufp
	for {
		n, addr, err := c.PacketConn.ReadFrom(buf)
		if err != nil {
			return 0, addr, err
		}
		out, derr := c.obfs.deobfuscate(buf[:n], p)
		if derr != nil {
			continue
		}
		return out, addr, nil
	}
}

func (c *obfsPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	bufp := c.pool.Get().(*[]byte)
	defer c.pool.Put(bufp)
	buf := *bufp
	n, err := c.obfs.obfuscate(p, buf)
	if err != nil {
		return 0, err
	}
	if _, werr := c.PacketConn.WriteTo(buf[:n], addr); werr != nil {
		return 0, werr
	}
	return len(p), nil
}
