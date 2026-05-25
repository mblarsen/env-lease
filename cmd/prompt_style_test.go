package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStyleConfirmPromptPreservesGrantPromptText(t *testing.T) {
	plain := "Grant 'GOOGLE_API_KEY'? [y/n/a/d/?]: "

	styled := styleConfirmPrompt("Grant 'GOOGLE_API_KEY'?")

	assert.Equal(t, plain, stripANSI(styled))
	assert.Contains(t, styled, "\x1b[")
	assert.Contains(t, styled, "GOOGLE_API_KEY")
	assert.Contains(t, styled, "[y/n/a/d/?]")
}

func TestStyleConfirmPromptPreservesGenericPromptText(t *testing.T) {
	plain := "Prompt 1? [y/n/a/d/?]: "

	styled := styleConfirmPrompt("Prompt 1?")

	assert.Equal(t, plain, stripANSI(styled))
}
