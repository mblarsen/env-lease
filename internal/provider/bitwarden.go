package provider

import (
	"encoding/json"
	"fmt"
	"log/slog"
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

	// If an organization is specified, we must first look up the item's ID
	// as `bw get` doesn't scope name searches by organization.
	itemID := document
	if p.OrganizationID != "" {
		var err error
		slog.Debug("Bitwarden: organization ID present, resolving name to ID", "name", document)
		itemID, err = p.findItemID(document)
		if err != nil {
			return "", err
		}
	}

	args := []string{"get"}
	if field == "" {
		args = append(args, "item", itemID)
	} else {
		args = append(args, field, itemID)
	}
	slog.Debug("Bitwarden: executing final get command", "args", args)
	cmd := execCommand("bw", args...)

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

// findItemID finds the unique ID of a Bitwarden item by its name.
//
// **NOTE ON BITWARDEN CLI BUG/QUIRK:**
// As of October 2025, the Bitwarden CLI (`bw`) has an unintuitive and buggy
// interaction between the `--search` and `--organizationid` flags for the
// `list items` command.
//
//  1. `bw get item <name>` does NOT respect `--organizationid`, leading to
//     ambiguous results if items with the same name exist globally. See:
//     https://github.com/bitwarden/clients/issues/2736
//  2. `bw list items --search <name> --organizationid <id>` does NOT work as
//     expected. It appears to perform the search globally *before* filtering,
//     often returning an empty set for the organization even if the item exists.
//
// To work around this, the only reliable method is to fetch ALL items for the
// given organization and then perform the search/filter for the exact name
// within this Go function. This is less efficient but necessary for correctness.
//
// A fallback global search is included for items not associated with an organization.
func (p *BitwardenCLI) findItemID(name string) (string, error) {
	slog.Debug("Bitwarden: finding item ID", "name", name, "organizationID", p.OrganizationID)
	args := []string{"list", "items", "--raw"}
	if p.OrganizationID != "" {
		args = append(args, "--organizationid", p.OrganizationID)
	}
	slog.Debug("Bitwarden: executing command", "args", args)
	cmd := execCommand("bw", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to list bitwarden items: %s", string(output))
	}

	var items []bwItem
	if err := json.Unmarshal(output, &items); err != nil {
		return "", fmt.Errorf("failed to parse bitwarden items: %w", err)
	}
	slog.Debug("Bitwarden: initial item list returned", "count", len(items))

	var foundItems []bwItem
	for _, item := range items {
		if item.Name == name {
			foundItems = append(foundItems, item)
		}
	}
	slog.Debug("Bitwarden: found items after filtering by exact name", "count", len(foundItems))

	if len(foundItems) == 0 {
		// If we didn't find it, try a global search as a fallback for non-org items
		if p.OrganizationID == "" {
			slog.Debug("Bitwarden: item not found, falling back to global search")
			args = []string{"list", "items", "--search", name, "--raw"}
			cmd = execCommand("bw", args...)
			output, err = cmd.CombinedOutput()
			if err != nil {
				return "", fmt.Errorf("failed to list bitwarden items: %s", string(output))
			}
			if err := json.Unmarshal(output, &items); err != nil {
				return "", fmt.Errorf("failed to parse bitwarden items: %w", err)
			}
			slog.Debug("Bitwarden: fallback search returned", "count", len(items))
			for _, item := range items {
				if item.Name == name {
					foundItems = append(foundItems, item)
				}
			}
			slog.Debug("Bitwarden: found items after fallback filtering", "count", len(foundItems))
		}
	}

	if len(foundItems) == 0 {
		return "", fmt.Errorf("item '%s' not found", name)
	}
	if len(foundItems) > 1 {
		return "", fmt.Errorf("multiple items named '%s' found, please use ID", name)
	}
	slog.Debug("Bitwarden: found unique item ID", "id", foundItems[0].ID)
	return foundItems[0].ID, nil
}

// fetchAttachment retrieves an attachment from Bitwarden.
func (p *BitwardenCLI) fetchAttachment(source string) (string, error) {
	parts := strings.SplitN(strings.TrimPrefix(source, "bw+file://"), "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid bw+file URI format: %s", source)
	}
	itemName, attachmentName := parts[0], parts[1]

	itemID, err := p.findItemID(itemName)
	if err != nil {
		return "", err
	}

	// We need to fetch the full item details to get the attachment list
	cmd := execCommand("bw", "get", "item", itemID, "--raw")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get item details: %s", string(output))
	}

	var item bwItem
	if err := json.Unmarshal(output, &item); err != nil {
		return "", fmt.Errorf("failed to parse item details: %w", err)
	}

	for _, attachment := range item.Attachments {
		if attachment.FileName == attachmentName {
			args := []string{"get", "attachment", attachmentName, "--itemid", item.ID, "--raw"}
			cmd := execCommand("bw", args...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				return "", fmt.Errorf("failed to fetch bitwarden attachment: %s", string(output))
			}
			return strings.TrimSpace(string(output)), nil
		}
	}

	return "", fmt.Errorf("attachment '%s' not found in item '%s'", attachmentName, itemName)
}
