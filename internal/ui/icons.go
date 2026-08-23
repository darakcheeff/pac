package ui

import (
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

const splitHorizontalSVG = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 20 20">
  <rect x="2" y="2" width="16" height="16" rx="2" fill="#eeeeec" stroke="#555753" stroke-width="1.8"/>
  <line x1="2" y1="10" x2="18" y2="10" stroke="#555753" stroke-width="1.8"/>
  <rect x="4" y="4.5" width="4" height="2" rx="0.5" fill="#4e9a06"/>
  <rect x="4" y="12.5" width="4" height="2" rx="0.5" fill="#3465a4"/>
</svg>`

const splitVerticalSVG = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 20 20">
  <rect x="2" y="2" width="16" height="16" rx="2" fill="#eeeeec" stroke="#555753" stroke-width="1.8"/>
  <line x1="10" y1="2" x2="10" y2="18" stroke="#555753" stroke-width="1.8"/>
  <rect x="4" y="4.5" width="2" height="3" rx="0.5" fill="#4e9a06"/>
  <rect x="12" y="4.5" width="2" height="3" rx="0.5" fill="#3465a4"/>
</svg>`

// GetSplitHorizontalImage returns a crisp GTK Image representing top/bottom screen split
func GetSplitHorizontalImage() *gtk.Image {
	return imageFromSVG(splitHorizontalSVG, "view-split-top-bottom-symbolic")
}

// GetSplitVerticalImage returns a crisp GTK Image representing left/right screen split
func GetSplitVerticalImage() *gtk.Image {
	return imageFromSVG(splitVerticalSVG, "view-split-left-right-symbolic")
}

func imageFromSVG(svgData string, fallbackIcon string) *gtk.Image {
	loader, err := gdk.PixbufLoaderNewWithType("svg")
	if err == nil {
		_, _ = loader.Write([]byte(svgData))
		_ = loader.Close()
		pixbuf, pErr := loader.GetPixbuf()
		if pErr == nil && pixbuf != nil {
			img, iErr := gtk.ImageNewFromPixbuf(pixbuf)
			if iErr == nil {
				return img
			}
		}
	}
	// Fallback to stock icon if SVG loader fails
	img, _ := gtk.ImageNewFromIconName(fallbackIcon, gtk.ICON_SIZE_LARGE_TOOLBAR)
	return img
}
