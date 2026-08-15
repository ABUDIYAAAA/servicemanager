package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// WriteNginxConfig writes the reverse proxy config for the given domain.
func WriteNginxConfig(domain string, hostPort int) error {
	confContent := fmt.Sprintf(`server {
    listen 80;
    server_name %s;

    location / {
        proxy_pass http://127.0.0.1:%d;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
`, domain, hostPort)

	if runtime.GOOS == "darwin" {
		confPath := filepath.Join("/opt/homebrew/etc/nginx/servers", fmt.Sprintf("%s.conf", domain))
		return os.WriteFile(confPath, []byte(confContent), 0644)
	}

	// Ubuntu/Debian
	availablePath := filepath.Join("/etc/nginx/sites-available", fmt.Sprintf("%s.conf", domain))
	enabledPath := filepath.Join("/etc/nginx/sites-enabled", fmt.Sprintf("%s.conf", domain))

	// Try without sudo first (in case we run as root), if fail we can't do much using os.WriteFile directly.
	// We'll write to a temp file and use sudo mv to move it if needed.
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("%s.conf", domain))
	if err := os.WriteFile(tmpFile, []byte(confContent), 0644); err != nil {
		return err
	}

	cmd := exec.Command("sudo", "mv", tmpFile, availablePath)
	if err := cmd.Run(); err != nil {
		// fallback to direct write
		if err2 := os.WriteFile(availablePath, []byte(confContent), 0644); err2 != nil {
			return fmt.Errorf("failed to write nginx config: %v (sudo failed with %v)", err2, err)
		}
	}

	// Create symlink
	symCmd := exec.Command("sudo", "ln", "-sf", availablePath, enabledPath)
	if err := symCmd.Run(); err != nil {
		// fallback to direct symlink
		_ = os.Symlink(availablePath, enabledPath)
	}

	return nil
}

// ReloadNginx executes the reload command.
func ReloadNginx() error {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("nginx", "-s", "reload")
	} else {
		cmd = exec.Command("sudo", "nginx", "-s", "reload") // Using sudo for Linux prod
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to reload nginx: %s %v", string(output), err)
	}
	return nil
}

// SetupTLS runs Certbot for the given domain.
func SetupTLS(domain string) error {
	var cmd *exec.Cmd
	email := os.Getenv("CERTBOT_EMAIL")
	if email == "" {
		email = "admin@localhost"
	}
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("certbot", "--nginx", "-d", domain, "--non-interactive", "--agree-tos", "-m", email)
	} else {
		cmd = exec.Command("sudo", "certbot", "--nginx", "-d", domain, "--non-interactive", "--agree-tos", "-m", email)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("certbot failed: %s %v", string(output), err)
	}
	return nil
}
