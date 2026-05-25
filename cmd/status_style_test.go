package cmd

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

func stripANSI(s string) string {
	return ansi.Strip(s)
}

func TestStyleStatusOutputPreservesTextLayout(t *testing.T) {
	plain := "VARIABLE           SOURCE                             DESTINATION       EXPIRES IN\n" +
		"<exploded>         op+file://container_env.json      /Users/mbl/.env   59m33s\n" +
		" ├─ ORY_API_URL                                       /Users/mbl/.env   14h30m5s\n" +
		" └─ ORY_SCHEMA_ID                                     /Users/mbl/.env   59m33s\n"

	styled := styleStatusOutput(plain)

	assert.Equal(t, plain, stripANSI(styled))
	assert.Contains(t, styled, "\x1b[")
}

func TestStyleStatusOutputStylesRequestedSegments(t *testing.T) {
	plain := "VARIABLE   SOURCE                                  DESTINATION       EXPIRES IN\n" +
		"<file>     op://Private/Openrouter/Crush key      /Users/mbl/.env   14h30m5s\n" +
		" └─ CHILD                                          /Users/mbl/.env   59m33s\n"

	styled := styleStatusOutput(plain)

	stripped := stripANSI(styled)
	assert.Contains(t, stripped, "VARIABLE")
	assert.Contains(t, stripped, "<file>")
	assert.Contains(t, stripped, "CHILD")
	assert.Contains(t, stripped, "op://Private/Openrouter/Crush key")
	assert.Contains(t, stripped, "└─")
	assert.Contains(t, stripped, "/Users/mbl/")
	assert.Contains(t, stripped, ".env")
	assert.Contains(t, stripped, "14h30m5s")
	assert.Contains(t, styled, "14")
	assert.Contains(t, styled, "h")
	assert.Contains(t, styled, "30")
	assert.Contains(t, styled, "m")
	assert.Contains(t, styled, "5")
	assert.Contains(t, styled, "s")
	assert.Equal(t, plain, stripped)
}

func TestStyleStatusOutputStylesExplodedPlaceholder(t *testing.T) {
	plain := "VARIABLE     SOURCE          DESTINATION       EXPIRES IN\n" +
		"<exploded>   source          /Users/mbl/.env   59m33s\n"

	styled := styleStatusOutput(plain)

	assert.Equal(t, plain, stripANSI(styled))
	assert.Contains(t, styled, "<exploded>")
	assert.Contains(t, styled, "\x1b[1;38;2;56;189;248m<exploded>")
}

func TestStyleStatusOutputDoesNotTreatSourceSchemeSlashesAsPath(t *testing.T) {
	plain := "VARIABLE     SOURCE                                              DESTINATION       EXPIRES IN\n" +
		"<exploded>   op+file://app-iac container env/container_env.json   /Users/mbl/.env   59m33s\n"

	styled := styleStatusOutput(plain)

	assert.Equal(t, plain, stripANSI(styled))
	assert.Contains(t, styled, "op+file")
	assert.Contains(t, styled, "://")
}
