package ci_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/yersonargotev/packy/internal/testprocess"
)

// catalogGitOrigin serves one local Git repository over verified HTTPS through
// a loopback-only proxy. No request is forwarded to a live origin service.
func catalogGitOrigin(t *testing.T, downloads ...*atomic.Int32) (commit, proxyURL, certificateFile string) {
	t.Helper()
	origin := t.TempDir()
	environment := testprocess.Env(t)
	if err := os.Mkdir(filepath.Join(origin, "skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(origin, "skill", "SKILL.md"), []byte("upstream skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "Origin fixture"}} {
		_, stderr, err := runCatalogCLI(t, environment, origin, "git", args...)
		if err != nil {
			t.Fatalf("origin setup: %v %s", err, stderr)
		}
	}
	head, stderr, err := runCatalogCLI(t, environment, origin, "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("origin identity: %v %s", err, stderr)
	}
	commit = string(head[:len(head)-1])
	if err := os.WriteFile(filepath.Join(origin, "skill", "SKILL.md"), []byte("unselected later revision\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", "."},
		{"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "Unselected later revision"},
		{"config", "uploadpack.allowReachableSHA1InWant", "true"},
	} {
		if _, stderr, err := runCatalogCLI(t, environment, origin, "git", args...); err != nil {
			t.Fatalf("historical origin setup: %v %s", err, stderr)
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Controlled Catalog origin"}, DNSNames: []string{"github.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true, IsCA: true}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificateFile = filepath.Join(t.TempDir(), "origin-ca.pem")
	if err := os.WriteFile(certificateFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		args := []string{"upload-pack", "--stateless-rpc"}
		switch r.URL.Path {
		case "/example/upstream.git/info/refs":
			args = append(args, "--advertise-refs")
			w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
		case "/example/upstream.git/git-upload-pack":
			if len(downloads) > 0 {
				downloads[0].Add(1)
			}
			w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				return
			}
			request := packp.NewUploadPackRequest()
			if err := request.Decode(bytes.NewReader(body)); err != nil {
				t.Errorf("decode requested origin revision: %v", err)
				return
			}
			if len(request.Wants) != 1 || request.Wants[0] != plumbing.NewHash(commit) || request.Depth != packp.DepthCommits(1) {
				t.Errorf("origin acquisition must request only pinned commit %s at depth one; wants=%v depth=%v", commit, request.Wants, request.Depth)
				http.Error(w, "origin acquisition exceeded requested revision", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		default:
			http.NotFound(w, r)
			return
		}
		args = append(args, origin)
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, "git", args...)
		command.Env = environment
		command.Stdin = r.Body
		output, err := command.Output()
		if err != nil {
			t.Errorf("controlled Git origin: %v", err)
			http.Error(w, "fixture Git failure", 500)
			return
		}
		if r.URL.Path == "/example/upstream.git/info/refs" {
			_, _ = fmt.Fprint(w, "001e# service=git-upload-pack\n0000")
		}
		_, _ = w.Write(output)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12}
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	t.Cleanup(server.Close)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != "github.com:443" {
			http.Error(w, "unsupported origin", http.StatusForbidden)
			return
		}
		upstream, err := net.DialTimeout("tcp", server.Listener.Addr().String(), 5*time.Second)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer upstream.Close()
		client, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer client.Close()
		_ = client.SetDeadline(time.Now().Add(15 * time.Second))
		_ = upstream.SetDeadline(time.Now().Add(15 * time.Second))
		if _, err := fmt.Fprint(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}
		done := make(chan struct{})
		go func() { _, _ = io.Copy(upstream, client); _ = upstream.Close(); close(done) }()
		_, _ = io.Copy(client, upstream)
		_ = client.Close()
		<-done
	}))
	t.Cleanup(proxy.Close)
	return commit, proxy.URL, certificateFile
}
