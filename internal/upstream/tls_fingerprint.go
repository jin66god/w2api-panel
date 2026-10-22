package upstream

import (
	"context"
	"crypto/tls"
	"net"
	"os"
	"strings"

	utls "github.com/refraction-networking/utls"
)

// 官方 WorkBuddy 业务 API 的 ClientHello 指纹（2026-10-22 真机抓包，CN 与国际版同一套）。
//
//	JA4(CN)    t13d1812h1_5d04281c6031_ef7df7f74e48
//	JA4(global) t13d1812h1_5d04281c6031_ef7df7f74e48
//	JA3        771,4865-4866-4867-49199-...,0-23-65281-10-11-35-16-13-51-45-43-21,29-23-24,0
//
// 对照基线：Go 标准库默认是 t13d131100_f57a46bbacb6_f50d94e863eb（13 cipher / 11 扩展 /
// 无 ALPN），与官方三项全不同，属于可被 JA4 一次筛出的特征。
//
// 关闭方式：WB2_TLS_FINGERPRINT=0（回退到标准库 http.Transport）。
// 注意：走 HTTPS_PROXY 时 http.Transport 用 DialContext 建隧道、TLS 在隧道内由标准库完成，
// 此时 uTLS 不生效，指纹仍是 Go 默认。
func tlsFingerprintEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("WB2_TLS_FINGERPRINT")))
	return !(v == "0" || v == "false" || v == "off" || v == "no")
}

// 顺序严格照抄抓包（JA3 ciphers 段），不要重排：cipher 顺序参与 JA3/JA4 计算。
var officialCipherSuites = []uint16{
	0x1301, // TLS_AES_128_GCM_SHA256
	0x1302, // TLS_AES_256_GCM_SHA384
	0x1303, // TLS_CHACHA20_POLY1305_SHA256
	0xC02F, // TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
	0xC02B, // TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
	0xC030, // TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
	0xC02C, // TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
	0xC027, // TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256
	0xCCA9, // TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256
	0xCCA8, // TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256
	0xC009, // TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA
	0xC013, // TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA
	0xC00A, // TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA
	0xC014, // TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA
	0x009C, // TLS_RSA_WITH_AES_128_GCM_SHA256
	0x009D, // TLS_RSA_WITH_AES_256_GCM_SHA384
	0x002F, // TLS_RSA_WITH_AES_128_CBC_SHA
	0x0035, // TLS_RSA_WITH_AES_256_CBC_SHA
}

// 官方 ClientHello 的扩展顺序：SNI / EMS / renegotiation_info / groups / point_formats /
// session_ticket / ALPN / sig_algs / key_share / psk_modes / supported_versions / padding。
// 注意 key_share 只带 x25519、ALPN 只有 http/1.1（官方业务端点不协商 h2）。
func officialClientHelloSpec() *utls.ClientHelloSpec {
	return &utls.ClientHelloSpec{
		CipherSuites:       officialCipherSuites,
		CompressionMethods: []byte{0},
		Extensions: []utls.TLSExtension{
			&utls.SNIExtension{},
			&utls.UtlsExtendedMasterSecretExtension{},
			&utls.RenegotiationInfoExtension{Renegotiation: utls.RenegotiateOnceAsClient},
			&utls.SupportedCurvesExtension{Curves: []utls.CurveID{utls.X25519, utls.CurveP256, utls.CurveP384}},
			&utls.SupportedPointsExtension{SupportedPoints: []byte{0}},
			&utls.SessionTicketExtension{},
			&utls.ALPNExtension{AlpnProtocols: []string{"http/1.1"}},
			&utls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []utls.SignatureScheme{
				utls.ECDSAWithP256AndSHA256,
				utls.PSSWithSHA256,
				utls.PKCS1WithSHA256,
				utls.PSSWithSHA384,
				utls.PKCS1WithSHA384,
				utls.PSSWithSHA512,
				utls.PKCS1WithSHA512,
				utls.PKCS1WithSHA1,
				utls.ECDSAWithSHA1,
				utls.ECDSAWithP384AndSHA384,
				utls.ECDSAWithP521AndSHA512,
			}},
			&utls.KeyShareExtension{KeyShares: []utls.KeyShare{{Group: utls.X25519}}},
			&utls.PSKKeyExchangeModesExtension{Modes: []uint8{utls.PskModeDHE}},
			&utls.SupportedVersionsExtension{Versions: []uint16{utls.VersionTLS13, utls.VersionTLS12}},
			&utls.UtlsPaddingExtension{GetPaddingLen: utls.BoringPaddingStyle},
		},
	}
}

// newUTLSDialer 返回 DialTLSContext 实现：对直连 HTTPS 使用官方 ClientHello 指纹。
// 握手在此完成，http.Transport 不会二次握手；连接池/超时仍由 Transport 管理。
func newUTLSDialer(dialer *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		raw, err := dialer.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			raw.Close()
			return nil, err
		}
		conn := utls.UClient(raw, &utls.Config{
			ServerName: host,
			NextProtos: []string{"http/1.1"},
			MinVersion: tls.VersionTLS12,
		}, utls.HelloCustom)
		if err := conn.ApplyPreset(officialClientHelloSpec()); err != nil {
			raw.Close()
			return nil, err
		}
		if err := conn.HandshakeContext(ctx); err != nil {
			raw.Close()
			return nil, err
		}
		return conn, nil
	}
}
