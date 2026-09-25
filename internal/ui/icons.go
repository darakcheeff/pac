package ui

import (
	"fmt"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

const splitHorizontalSVG = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 20 20">
  <g transform="rotate(90 10 10)">
    <rect x="2" y="2" width="16" height="16" rx="2" fill="#eeeeec" stroke="#555753" stroke-width="1.8"/>
    <path d="M 10 2 L 16 2 A 2 2 0 0 1 18 4 L 18 16 A 2 2 0 0 1 16 18 L 10 18 Z" fill="#555753"/>
    <line x1="10" y1="2" x2="10" y2="18" stroke="#555753" stroke-width="1.8"/>
  </g>
</svg>`

const splitVerticalSVG = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 20 20">
  <rect x="2" y="2" width="16" height="16" rx="2" fill="#eeeeec" stroke="#555753" stroke-width="1.8"/>
  <path d="M 10 2 L 16 2 A 2 2 0 0 1 18 4 L 18 16 A 2 2 0 0 1 16 18 L 10 18 Z" fill="#555753"/>
  <line x1="10" y1="2" x2="10" y2="18" stroke="#555753" stroke-width="1.8"/>
</svg>`

const unsplitSVG = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 20 20">
  <rect x="2" y="2.5" width="7" height="15" rx="1.8" fill="#555753"/>
  <polygon points="3.5,10 7.5,6.5 7.5,13.5" fill="#eeeeec"/>
  <rect x="11" y="2.5" width="7" height="15" rx="1.8" fill="#555753"/>
  <polygon points="16.5,10 12.5,6.5 12.5,13.5" fill="#eeeeec"/>
</svg>`

const downloadSVGTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<svg height="16px" viewBox="0 0 16 16" width="16px" xmlns="http://www.w3.org/2000/svg">
  <path d="m 8 1 c -0.55 0 -1 0.45 -1 1 v 7.586 l -2.293 -2.293 c -0.39 -0.39 -1.023 -0.39 -1.414 0 s -0.39 1.023 0 1.414 l 4 4 c 0.39 0.39 1.023 0.39 1.414 0 l 4 -4 c 0.39 -0.39 0.39 -1.023 0 -1.414 s -1.023 -0.39 -1.414 0 l -2.293 2.293 v -7.586 c 0 -0.55 -0.45 -1 -1 -1 z m -7 13 v 2 h 14 v -2 z" fill="%s"/>
</svg>`

const uploadSVGTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<svg height="16px" viewBox="0 0 16 16" width="16px" xmlns="http://www.w3.org/2000/svg">
  <path d="m 8 13 c 0.55 0 1 -0.45 1 -1 v -7.586 l 2.293 2.293 c 0.39 0.39 1.023 0.39 1.414 0 s 0.39 -1.023 0 -1.414 l -4 -4 c -0.39 -0.39 -1.023 -0.39 -1.414 0 l -4 4 c -0.39 0.39 -0.39 1.023 0 1.414 s 1.023 0.39 1.414 0 l 2.293 -2.293 v 7.586 c 0 0.55 0.45 1 1 1 z m -7 1 v 2 h 14 v -2 z" fill="%s"/>
</svg>`

// Classic crescent moon 🌙 matching emoji
const moonSVG = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24">
  <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" fill="#e5a50a"/>
</svg>`

// Radiant Sun ☀️ with glowing center and rays
const sunSVG = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#f5c211" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
  <circle cx="12" cy="12" r="4.5" fill="#f5c211"/>
  <line x1="12" y1="1" x2="12" y2="3.5"/>
  <line x1="12" y1="20.5" x2="12" y2="23"/>
  <line x1="4.22" y1="4.22" x2="5.99" y2="5.99"/>
  <line x1="18.01" y1="18.01" x2="19.78" y2="19.78"/>
  <line x1="1" y1="12" x2="3.5" y2="12"/>
  <line x1="20.5" y1="12" x2="23" y2="12"/>
  <line x1="4.22" y1="19.78" x2="5.99" y2="18.01"/>
  <line x1="18.01" y1="5.99" x2="19.78" y2="4.22"/>
</svg>`

// GetSplitHorizontalImage returns a crisp GTK Image representing top/bottom screen split
func GetSplitHorizontalImage() *gtk.Image {
	return imageFromSVG(splitHorizontalSVG, "view-split-top-bottom-symbolic")
}

// GetSplitVerticalImage returns a crisp GTK Image representing left/right screen split
func GetSplitVerticalImage() *gtk.Image {
	return imageFromSVG(splitVerticalSVG, "view-split-left-right-symbolic")
}

// GetUnsplitImage returns the unsplit/restore icon based on 03.png
func GetUnsplitImage() *gtk.Image {
	return imageFromSVG(unsplitSVG, "view-restore-symbolic")
}

// GetDownloadImage returns the download icon (arrow pointing into tray) with theme-adaptive contrast
func GetDownloadImage(isDark bool) *gtk.Image {
	color := "#2e3436"
	if isDark {
		color = "#eeeeee"
	}
	return imageFromSVG(fmt.Sprintf(downloadSVGTemplate, color), "document-save-symbolic")
}

// GetUploadImage returns the upload icon (arrow pointing up from tray) with theme-adaptive contrast
func GetUploadImage(isDark bool) *gtk.Image {
	color := "#2e3436"
	if isDark {
		color = "#eeeeee"
	}
	return imageFromSVG(fmt.Sprintf(uploadSVGTemplate, color), "document-send-symbolic")
}

// GetMoonImage returns an unmistakable crescent moon 🌙 image
func GetMoonImage() *gtk.Image {
	return imageFromSVG(moonSVG, "weather-clear-night-symbolic")
}

// GetSunImage returns a clear sun ☀️ image
func GetSunImage() *gtk.Image {
	return imageFromSVG(sunSVG, "weather-clear-symbolic")
}

const doorExitSVGTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<svg height="18px" viewBox="0 0 20 20" width="18px" xmlns="http://www.w3.org/2000/svg">
  <path d="M3 2 C2 2 1 3 1 4 L1 16 C1 17 2 18 3 18 L10 18 C11 18 12 17 12 16 L12 14.5 C12 14 11.5 13.5 11 13.5 C10.5 13.5 10 14 10 14.5 L10 16 L3 16 L3 4 L10 4 L10 5.5 C10 6 10.5 6.5 11 6.5 C11.5 6.5 12 6 12 5.5 L12 4 C12 3 11 2 10 2 Z" fill="%s"/>
  <path d="M7 9 C6.4 9 6 9.4 6 10 C6 10.6 6.4 11 7 11 L15 11 L15 13 C15 13.4 15.4 13.8 15.8 13.6 L19.4 10.6 C19.8 10.3 19.8 9.7 19.4 9.4 L15.8 6.4 C15.4 6.2 15 6.6 15 7 L15 9 Z" fill="#e55353"/>
</svg>`

// GetDoorExitImage returns a door exit icon with theme-adaptive contrast and red exit arrow
func GetDoorExitImage(isDark bool) *gtk.Image {
	color := "#2e3436"
	if isDark {
		color = "#eeeeee"
	}
	return imageFromSVG(fmt.Sprintf(doorExitSVGTemplate, color), "application-exit-symbolic")
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
	img, _ := gtk.ImageNewFromIconName(fallbackIcon, gtk.ICON_SIZE_BUTTON)
	return img
}
