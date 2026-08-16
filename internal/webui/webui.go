package webui

import "embed"

// Dist is replaced by the React production build in the Docker build stage.
//
//go:embed dist/*
var Dist embed.FS
