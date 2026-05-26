package cmd

import "github.com/mblarsen/env-lease/internal/presentation"

var presenter presentation.Renderer = presentation.NewTerminal()
