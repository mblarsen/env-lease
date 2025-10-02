package provider

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "No command\n")
		os.Exit(1)
	}

	cmd, args := args[0], args[1:]
	if cmd == "bw" {
		subcommand := args[0]
		args = args[1:]

		switch subcommand {
		case "get":
			getWhat := args[0]
			args = args[1:]
			switch getWhat {
			case "item":
				doc := args[0]
				if doc == "my-item-id" || doc == "org-item-id-456" || doc == "item-id-123" {
					fmt.Fprintf(os.Stdout, `{"id":"%s","name":"%s","attachments":[{"fileName":"my-attachment.txt"}]}`, doc, doc)
				} else {
					fmt.Fprint(os.Stderr, "item not found")
					os.Exit(1)
				}
			case "password":
				doc := args[0]
				switch doc {
				case "my-item-id":
					fmt.Fprint(os.Stdout, "my-secret")
				case "org-item-id-456":
					fmt.Fprint(os.Stdout, "org-secret")
				case "not-found":
					fmt.Fprint(os.Stderr, "not found")
					os.Exit(1)
				case "locked":
					fmt.Fprint(os.Stderr, "vault is locked")
					os.Exit(1)
				}
			case "attachment":
				attachmentName := args[0]
				args = args[1:]
				var itemID string
				for i, arg := range args {
					if arg == "--itemid" && i+1 < len(args) {
						itemID = args[i+1]
						break
					}
				}
				if attachmentName == "my-attachment.txt" && (itemID == "item-id-123" || itemID == "org-item-id-456") {
					fmt.Fprint(os.Stdout, "attachment content")
				} else {
					fmt.Fprint(os.Stderr, "attachment not found")
					os.Exit(1)
				}
			}
		case "list":
			listWhat := args[0]
			args = args[1:]
			if listWhat == "items" {
				var search, orgID string
				for i, arg := range args {
					if arg == "--search" && i+1 < len(args) {
						search = args[i+1]
					}
					if arg == "--organizationid" && i+1 < len(args) {
						orgID = args[i+1]
					}
				}

				if orgID == "org-id-123" {
					// Return all items for the org; filtering happens in the Go code.
					fmt.Fprint(os.Stdout, `[
						{"id":"org-item-id-456","name":"OrgItem"},
						{"id":"another-org-item","name":"AnotherOrgItem"}
					]`)
				} else if search == "my-doc" {
					fmt.Fprint(os.Stdout, `[{"id":"item-id-123","name":"my-doc"}]`)
				} else {
					fmt.Fprint(os.Stdout, `[]`)
				}
			}
		default:
			fmt.Fprintf(os.Stderr, "Unknown bw subcommand %q\n", subcommand)
			os.Exit(1)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Unknown command %q\n", cmd)
		os.Exit(1)
	}
}

func TestBitwardenCLIFetch(t *testing.T) {
	execCommand = func(command string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", command}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		return cmd
	}
	defer func() { execCommand = exec.Command }()

	t.Run("successful fetch with field", func(t *testing.T) {
		source := "bw://my-item-id/password"
		provider := &BitwardenCLI{}

		secret, err := provider.Fetch(source)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if secret != "my-secret" {
			t.Errorf("expected secret to be 'my-secret', got '%s'", secret)
		}
	})

	t.Run("successful fetch of item", func(t *testing.T) {
		source := "bw://my-item-id"
		provider := &BitwardenCLI{}

		secret, err := provider.Fetch(source)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		expectedSecret := `{"id":"my-item-id","name":"my-item-id","attachments":[{"fileName":"my-attachment.txt"}]}`
		if secret != expectedSecret {
			t.Errorf("expected secret to be '%s', got '%s'", expectedSecret, secret)
		}
	})

	t.Run("successful fetch with organization id", func(t *testing.T) {
		source := "bw://OrgItem/password"
		provider := &BitwardenCLI{OrganizationID: "org-id-123"}

		secret, err := provider.Fetch(source)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if secret != "org-secret" {
			t.Errorf("expected secret to be 'org-secret', got '%s'", secret)
		}
	})

	t.Run("successful attachment fetch", func(t *testing.T) {
		source := "bw+file://my-doc/my-attachment.txt"
		provider := &BitwardenCLI{}
		secret, err := provider.Fetch(source)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if secret != "attachment content" {
			t.Errorf("expected secret to be 'attachment content', got '%s'", secret)
		}
	})

	t.Run("item not found", func(t *testing.T) {
		source := "bw://not-found/password"
		provider := &BitwardenCLI{}

		_, err := provider.Fetch(source)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected error to contain 'not found', got '%s'", err.Error())
		}
	})

	t.Run("vault is locked", func(t *testing.T) {
		source := "bw://locked/password"
		provider := &BitwardenCLI{}

		_, err := provider.Fetch(source)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "vault is locked") {
			t.Errorf("expected error to contain 'vault is locked', got '%s'", err.Error())
		}
	})

	t.Run("invalid scheme", func(t *testing.T) {
		source := "http://my-item-id"
		provider := &BitwardenCLI{}

		_, err := provider.Fetch(source)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		expectedError := "invalid scheme: http"
		if err.Error() != expectedError {
			t.Errorf("expected error to be '%s', got '%s'", expectedError, err.Error())
		}
	})

	t.Run("document not found in source URI", func(t *testing.T) {
		source := "bw://"
		provider := &BitwardenCLI{}

		_, err := provider.Fetch(source)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		expectedError := "document not found in source URI"
		if err.Error() != expectedError {
			t.Errorf("expected error to be '%s', got '%s'", expectedError, err.Error())
		}
	})

	t.Run("attachment not found", func(t *testing.T) {
		source := "bw+file://my-doc/non-existent.txt"
		provider := &BitwardenCLI{}

		_, err := provider.Fetch(source)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		expectedError := "attachment 'non-existent.txt' not found in item 'my-doc'"
		if err.Error() != expectedError {
			t.Errorf("expected error to be '%s', got '%s'", expectedError, err.Error())
		}
	})
}
