package system

import (
	"os"
	"strings"
)

// InContainer reports whether the process runs inside the official container
// layout (ALX_CONTAINER=1, or ALEMONJS_SETUP_ROOTS pointing under /app).
// Container deployments cannot be fixed through macOS system settings; the
// mounted host directories must be writable by the container user instead, so
// permission advice needs a different wording inside the image.
func InContainer() bool {
	if strings.TrimSpace(os.Getenv("ALX_CONTAINER")) != "" {
		return true
	}
	roots := strings.TrimSpace(os.Getenv("ALEMONJS_SETUP_ROOTS"))
	return strings.HasPrefix(roots, "/app/") || strings.Contains(roots, ":/app/")
}
