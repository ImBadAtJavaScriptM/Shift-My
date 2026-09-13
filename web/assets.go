package web

import "embed"

// Assets contains the dashboard template and static assets.
//
//go:embed templates/index.html static/*
var Assets embed.FS
