package provider

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

var execCommand = exec.Command

// BitwardenCLI is a secret provider that uses the Bitwarden CLI to fetch secrets.
type BitwardenCLI struct {
	OrganizationID string
}

// NewBitwardenCLI creates a new BitwardenCLI provider.
func NewBitwardenCLI() (*BitwardenCLI, error) {
	if _, err := exec.LookPath("bw"); err != nil {
		return nil, fmt.Errorf("bitwarden-cli not found in PATH")
	}
	return &BitwardenCLI{}, nil
}

// Fetch retrieves a secret from Bitwarden using the Bitwarden CLI.
func (p *BitwardenCLI) Fetch(source string) (string, error) {
	if strings.HasPrefix(source, "bw+file://") {
		return p.fetchAttachment(source)
	}
	return p.fetchField(source)
}

// fetchField retrieves a secret field from Bitwarden.
func (p *BitwardenCLI) fetchField(source string) (string, error) {
	u, err := url.Parse(source)
	if err != nil {
		return "", fmt.Errorf("invalid source URI: %w", err)
	}

	if u.Scheme != "bw" {
		return "", fmt.Errorf("invalid scheme: %s", u.Scheme)
	}

	document := u.Host
	if document == "" {
		return "", fmt.Errorf("document not found in source URI")
	}

	field := strings.TrimPrefix(u.Path, "/")

	var cmd *exec.Cmd
	args := []string{"get"}
	if field == "" {
		args = append(args, "item", document)
	} else {
		args = append(args, field, document)
	}
	if p.OrganizationID != "" {
		args = append(args, "--organizationid", p.OrganizationID)
	}
	cmd = execCommand("bw", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to fetch secret from bitwarden: %s", string(output))
	}

	return strings.TrimSpace(string(output)), nil
}

// bwItem represents a Bitwarden item.
type bwItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Attachments []struct {
		FileName string `json:"fileName"`
	} `json:"attachments"`
}

// fetchAttachment retrieves an attachment from Bitwarden.
func (p *BitwardenCLI) fetchAttachment(source string) (string, error) {
	parts := strings.SplitN(strings.TrimPrefix(source, "bw+file://"), "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid bw+file URI format: %s", source)
	}
	itemName, attachmentName := parts[0], parts[1]

	args := []string{"list", "items", "--search", itemName, "--raw"}
	if p.OrganizationID != "" {
		args = append(args, "--organizationid", p.OrganizationID)
	}
	cmd := execCommand("bw", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to list bitwarden items: %s", string(output))
	}

	var items []bwItem
	if err := json.Unmarshal(output, &items); err != nil {
		return "", fmt.Errorf("failed to parse bitwarden items: %w", err)
	}

	for _, item := range items {
		if item.Name == itemName {
			for _, attachment := range item.Attachments {
				if attachment.FileName == attachmentName {
					args := []string{"get", "attachment", attachmentName, "--itemid", item.ID, "--raw"}
					if p.OrganizationID != "" {
						args = append(args, "--organizationid", p.OrganizationID)
					}
					cmd := execCommand("bw", args...)
					output, err := cmd.CombinedOutput()
					if err != nil {
						return "", fmt.Errorf("failed to fetch bitwarden attachment: %s", string(output))
					}
					return strings.TrimSpace(string(output)), nil
				}
			}
		}
	}

	return "", fmt.Errorf("attachment '%s' not found in item '%s'", attachmentName, itemName)
}
