package outbound

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"

	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"
)

type Svnt struct {
	*Base
	option   *SvntOption
	auth     string
	instance string
	policyID string
	encrypt  int
	network  string
}

type SvntOption struct {
	BasicOption
	Name       string `proxy:"name"`
	Server     string `proxy:"server"`
	Port       int    `proxy:"port"`
	KeyData    string `proxy:"key-data"`
	PolicyID   string `proxy:"policy-id,omitempty"`
	Encrypt    int    `proxy:"encrypt,omitempty"`
	InstanceID string `proxy:"instance-id,omitempty"`
	Network    string `proxy:"network,omitempty"`
}

func NewSvnt(option SvntOption) (*Svnt, error) {
	if option.Server == "" {
		return nil, errors.New("server is required")
	}
	if option.Port <= 0 || option.Port > 65535 {
		return nil, fmt.Errorf("invalid port: %d", option.Port)
	}
	if strings.TrimSpace(option.KeyData) == "" {
		return nil, errors.New("key-data is required")
	}

	network := strings.TrimSpace(option.Network)
	if network == "" {
		network = "tcp"
	}
	if network != "tcp" {
		return nil, fmt.Errorf("svnt only supports tcp in this phase, got %q", network)
	}
	if option.Encrypt != 0 {
		return nil, fmt.Errorf("svnt only supports encrypt=0 in this phase, got %d", option.Encrypt)
	}

	auth, err := generateSVNTAuth(option.KeyData)
	if err != nil {
		return nil, err
	}

	instanceID := strings.TrimSpace(option.InstanceID)
	if instanceID == "" {
		instanceID = "mihomo"
	}

	policyID := strings.TrimSpace(option.PolicyID)
	if policyID == "" {
		policyID = "*"
	}

	outbound := &Svnt{
		Base: NewBase(BaseOption{
			Name:         option.Name,
			Addr:         net.JoinHostPort(option.Server, strconv.Itoa(option.Port)),
			Type:         C.Svnt,
			ProviderName: option.ProviderName,
			TFO:          option.TFO,
			MPTCP:        option.MPTCP,
			Interface:    option.Interface,
			RoutingMark:  option.RoutingMark,
			Prefer:       option.IPVersion,
		}),
		option:   &option,
		auth:     auth,
		instance: instanceID,
		policyID: policyID,
		encrypt:  option.Encrypt,
		network:  network,
	}
	outbound.dialer = option.NewDialer(outbound.DialOptions())
	return outbound, nil
}

func (s *Svnt) ProxyInfo() C.ProxyInfo {
	info := s.Base.ProxyInfo()
	info.DialerProxy = s.option.DialerProxy
	return info
}

func (s *Svnt) DialContext(ctx context.Context, metadata *C.Metadata) (_ C.Conn, err error) {
	if metadata == nil || !metadata.Valid() || metadata.DstPort == 0 {
		return nil, errors.New("invalid metadata for svnt outbound")
	}

	c, err := s.dialer.DialContext(ctx, s.network, s.addr)
	if err != nil {
		return nil, fmt.Errorf("%s connect error: %w", s.addr, err)
	}
	defer func(c net.Conn) {
		safeConnClose(c, err)
	}(c)

	if ctx.Done() != nil {
		done := N.SetupContextForConn(ctx, c)
		defer done(&err)
	}

	clientAddr := ""
	if metadata.SourceValid() {
		clientAddr = metadata.SourceAddress()
	}

	instancePath := []string{s.instance}
	ipPath := []string{c.LocalAddr().String()}
	rmsg, err := (&svntMsg{
		Magic:        svntMagic,
		Auth:         s.auth,
		InstanceID:   s.instance,
		MsgType:      svntMsgTypeTunnel,
		PolicyID:     s.policyID,
		Encrypt:      s.encrypt,
		TargetAddr:   metadata.RemoteAddress(),
		ClientAddr:   clientAddr,
		InstancePath: instancePath,
		IPPath:       ipPath,
	}).post(c)
	if err != nil {
		return nil, fmt.Errorf("%s connect error: %w", s.addr, err)
	}
	if rmsg.Status != svntMsgStatusOK {
		if rmsg.StatusString != "" {
			return nil, errors.New(rmsg.StatusString)
		}
		return nil, fmt.Errorf("svnt tunnel failed with status %d", rmsg.Status)
	}
	if rmsg.Encrypt != 0 {
		return nil, fmt.Errorf("svnt returned unsupported encrypt=%d", rmsg.Encrypt)
	}

	return NewConn(c, s), nil
}

func (s *Svnt) IsL3Protocol(metadata *C.Metadata) bool {
	return false
}

func (m *svntMsg) post(c io.ReadWriteCloser) (*svntMsg, error) {
	rmsg := &svntMsg{}
	if err := m.send(c); err != nil {
		return nil, err
	}
	if err := rmsg.read(c); err != nil {
		return nil, err
	}
	if rmsg.Auth != m.Auth {
		return nil, errors.New("消息验证不一致")
	}
	return rmsg, nil
}
