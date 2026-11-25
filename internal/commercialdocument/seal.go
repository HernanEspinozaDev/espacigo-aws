package commercialdocument

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"strings"
	"sync"
	"time"
)

var (
	oidData          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidSignedData    = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	oidSHA256        = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidRSAWithSHA256 = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 11}
)

type PlatformSeal struct {
	cert        *x509.Certificate
	key         *rsa.PrivateKey
	fingerprint string
}

var (
	defaultSealOnce sync.Once
	defaultSeal     *PlatformSeal
	defaultSealErr  error
)

func DefaultPlatformSeal() (*PlatformSeal, error) {
	defaultSealOnce.Do(func() {
		defaultSeal, defaultSealErr = newEphemeralPlatformSeal()
	})
	return defaultSeal, defaultSealErr
}

func newEphemeralPlatformSeal() (*PlatformSeal, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "EspaciGo Platform Seal (FES - Ley 19.799)",
			Organization: []string{"EspaciGo"},
			Country:      []string{"CL"},
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(cert.Raw)
	return &PlatformSeal{
		cert:        cert,
		key:         key,
		fingerprint: hex.EncodeToString(sum[:]),
	}, nil
}

func (s *PlatformSeal) CertFingerprint() string {
	return s.fingerprint
}

type cmsAlgorithmIdentifier struct {
	Algorithm  asn1.ObjectIdentifier
	Parameters asn1.RawValue `asn1:"optional"`
}

type cmsContentInfo struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue `asn1:"explicit,tag:0"`
}

type cmsEncapsulatedContentInfo struct {
	ContentType asn1.ObjectIdentifier
}

type cmsIssuerAndSerialNumber struct {
	Issuer       asn1.RawValue
	SerialNumber *big.Int
}

type cmsSignerInfo struct {
	Version            int
	SID                cmsIssuerAndSerialNumber
	DigestAlgorithm    cmsAlgorithmIdentifier
	SignatureAlgorithm cmsAlgorithmIdentifier
	Signature          []byte
}

type cmsSignedData struct {
	Version          int
	DigestAlgorithms []cmsAlgorithmIdentifier `asn1:"set"`
	EncapContentInfo cmsEncapsulatedContentInfo
	Certificates     asn1.RawValue   `asn1:"optional,tag:0"`
	SignerInfos      []cmsSignerInfo `asn1:"set"`
}

func (s *PlatformSeal) SignDetachedCMS(data []byte) ([]byte, error) {
	digest := sha256.Sum256(data)
	// Deterministic RSA PKCS#1 v1.5 signature (rand = nil) so PDF retries are byte-identical.
	var emptyRand io.Reader
	sig, err := rsa.SignPKCS1v15(emptyRand, s.key, crypto.SHA256, digest[:])
	if err != nil {
		return nil, err
	}
	sd := cmsSignedData{
		Version:          1,
		DigestAlgorithms: []cmsAlgorithmIdentifier{{Algorithm: oidSHA256}},
		EncapContentInfo: cmsEncapsulatedContentInfo{ContentType: oidData},
		Certificates: asn1.RawValue{
			Class:      asn1.ClassContextSpecific,
			Tag:        0,
			IsCompound: true,
			Bytes:      s.cert.Raw,
		},
		SignerInfos: []cmsSignerInfo{
			{
				Version: 1,
				SID: cmsIssuerAndSerialNumber{
					Issuer:       asn1.RawValue{FullBytes: s.cert.RawIssuer},
					SerialNumber: s.cert.SerialNumber,
				},
				DigestAlgorithm:    cmsAlgorithmIdentifier{Algorithm: oidSHA256},
				SignatureAlgorithm: cmsAlgorithmIdentifier{Algorithm: oidRSAWithSHA256},
				Signature:          sig,
			},
		},
	}
	sdBytes, err := asn1.Marshal(sd)
	if err != nil {
		return nil, err
	}
	ci := cmsContentInfo{
		ContentType: oidSignedData,
		Content: asn1.RawValue{
			Class:      asn1.ClassContextSpecific,
			Tag:        0,
			IsCompound: true,
			Bytes:      sdBytes,
		},
	}
	return asn1.Marshal(ci)
}

// SealPDF appends an ISO 32000 PKCS#7 detached signature dictionary to the PDF.
func SealPDF(pdfBytes []byte, seal *PlatformSeal, signedAt time.Time) ([]byte, error) {
	if seal == nil || len(pdfBytes) == 0 {
		return pdfBytes, nil
	}
	const contentsHexLen = 8192
	sigPlaceholder := strings.Repeat("0", contentsHexLen)
	byteRangePlaceholder := "[0 0000000000 0000000000 0000000000]"
	sigDict := fmt.Sprintf(
		"\n%%EspaciGo-FES-PKCS7-Seal\n<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter /adbe.pkcs7.detached /ByteRange %s /M (D:%sZ) /Reason (Firma Electronica Simple - Ley 19.799) /Location (EspaciGo Chile) /Contents <%s> >>\n",
		byteRangePlaceholder,
		signedAt.UTC().Format("20060102150405"),
		sigPlaceholder,
	)

	var out bytes.Buffer
	out.Write(pdfBytes)
	out.WriteString(sigDict)
	raw := out.Bytes()

	ltIdx := bytes.LastIndex(raw, []byte("<"+sigPlaceholder+">"))
	if ltIdx < 0 {
		return pdfBytes, nil
	}
	contentsStart := ltIdx
	contentsEnd := ltIdx + 1 + contentsHexLen + 1

	actualRange := fmt.Sprintf("[0 %010d %010d %010d]", contentsStart, contentsEnd, len(raw)-contentsEnd)
	brIdx := bytes.LastIndex(raw, []byte(byteRangePlaceholder))
	if brIdx < 0 || len(actualRange) != len(byteRangePlaceholder) {
		return pdfBytes, nil
	}
	copy(raw[brIdx:brIdx+len(actualRange)], actualRange)

	var signedBuf bytes.Buffer
	signedBuf.Write(raw[:contentsStart])
	signedBuf.Write(raw[contentsEnd:])
	cmsBytes, err := seal.SignDetachedCMS(signedBuf.Bytes())
	if err != nil {
		return nil, err
	}
	cmsHex := hex.EncodeToString(cmsBytes)
	if len(cmsHex) > contentsHexLen {
		return nil, fmt.Errorf("commercial document: cms signature exceeds placeholder")
	}
	copy(raw[contentsStart+1:contentsStart+1+len(cmsHex)], cmsHex)
	return raw, nil
}
