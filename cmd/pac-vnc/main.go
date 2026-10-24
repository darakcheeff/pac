package main

/*
#cgo pkg-config: gtk-vnc-2.0 gtk+-3.0
#include <gtk/gtk.h>
#include <gtk/gtkx.h>
#include <vncdisplay.h>
#include <stdlib.h>

typedef struct {
    char *username;
    char *password;
} AuthCreds;

static void on_vnc_auth_credential(VncDisplay *display, GValueArray *credList, gpointer user_data) {
    AuthCreds *creds = (AuthCreds*)user_data;
    if (!credList) return;
    for (guint i = 0; i < credList->n_values; i++) {
        GValue *val = g_value_array_get_nth(credList, i);
        int credType = g_value_get_enum(val);
        if (credType == VNC_DISPLAY_CREDENTIAL_PASSWORD && creds && creds->password) {
            vnc_display_set_credential(display, credType, creds->password);
        } else if (credType == VNC_DISPLAY_CREDENTIAL_USERNAME && creds && creds->username) {
            vnc_display_set_credential(display, credType, creds->username);
        }
    }
}

static void on_vnc_disconnected(VncDisplay *display, gpointer user_data) {
    (void)display;
    (void)user_data;
    gtk_main_quit();
}

static void on_vnc_server_cut_text(VncDisplay *display, const gchar *text, gpointer user_data) {
    (void)display;
    (void)user_data;
    if (text && *text) {
        GtkClipboard *cb = gtk_clipboard_get(GDK_SELECTION_CLIPBOARD);
        gtk_clipboard_set_text(cb, text, -1);
    }
}

static void on_clipboard_owner_change(GtkClipboard *cb, GdkEvent *event, gpointer user_data) {
    (void)event;
    VncDisplay *display = VNC_DISPLAY(user_data);
    gchar *text = gtk_clipboard_wait_for_text(cb);
    if (text) {
        vnc_display_client_cut_text(display, text);
        g_free(text);
    }
}

static void on_container_destroy(GtkWidget *widget, gpointer user_data) {
    (void)widget;
    (void)user_data;
    gtk_main_quit();
}

static int run_vnc(const char *host, int port, const char *username, const char *password,
                   unsigned long socket_xid, int scale, int keep_aspect, int view_only,
                   int shared, const char *title) {
    int argc = 0;
    char **argv = NULL;
    gtk_init(&argc, &argv);

    GtkWidget *container = NULL;
    if (socket_xid > 0) {
        container = gtk_plug_new((Window)socket_xid);
    } else {
        container = gtk_window_new(GTK_WINDOW_TOPLEVEL);
        gtk_window_set_title(GTK_WINDOW(container), title ? title : "PAC VNC Session");
        gtk_window_set_default_size(GTK_WINDOW(container), 1024, 768);
    }

    g_signal_connect(container, "destroy", G_CALLBACK(on_container_destroy), NULL);

    GtkWidget *vnc = vnc_display_new();
    if (!vnc) {
        return 1;
    }

    AuthCreds creds;
    creds.username = username ? g_strdup(username) : NULL;
    creds.password = password ? g_strdup(password) : NULL;

    if (scale) {
        vnc_display_set_scaling(VNC_DISPLAY(vnc), TRUE);
        vnc_display_set_smoothing(VNC_DISPLAY(vnc), TRUE);
        vnc_display_set_keep_aspect_ratio(VNC_DISPLAY(vnc), keep_aspect ? TRUE : FALSE);
    }
    if (view_only) {
        vnc_display_set_read_only(VNC_DISPLAY(vnc), TRUE);
    }
    if (shared) {
        vnc_display_set_shared_flag(VNC_DISPLAY(vnc), TRUE);
    }

    g_signal_connect(vnc, "vnc-auth-credential", G_CALLBACK(on_vnc_auth_credential), &creds);
    g_signal_connect(vnc, "vnc-disconnected", G_CALLBACK(on_vnc_disconnected), NULL);
    g_signal_connect(vnc, "vnc-server-cut-text", G_CALLBACK(on_vnc_server_cut_text), NULL);

    GtkClipboard *cb = gtk_clipboard_get(GDK_SELECTION_CLIPBOARD);
    g_signal_connect(cb, "owner-change", G_CALLBACK(on_clipboard_owner_change), vnc);

    GtkWidget *scrolled = gtk_scrolled_window_new(NULL, NULL);
    gtk_scrolled_window_set_policy(GTK_SCROLLED_WINDOW(scrolled), GTK_POLICY_AUTOMATIC, GTK_POLICY_AUTOMATIC);
    gtk_container_add(GTK_CONTAINER(scrolled), vnc);
    gtk_container_add(GTK_CONTAINER(container), scrolled);

    gtk_widget_show_all(container);

    gchar port_str[16];
    g_snprintf(port_str, sizeof(port_str), "%d", port);

    if (!vnc_display_open_host(VNC_DISPLAY(vnc), host, port_str)) {
        if (creds.username) g_free(creds.username);
        if (creds.password) g_free(creds.password);
        return 2;
    }

    gtk_main();

    if (creds.username) g_free(creds.username);
    if (creds.password) g_free(creds.password);
    return 0;
}
*/
import "C"

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"unsafe"
)

func main() {
	host := flag.String("host", "127.0.0.1", "VNC target host")
	port := flag.Int("port", 5900, "VNC target port")
	username := flag.String("user", "", "VNC username if required")
	password := flag.String("password", "", "VNC password")
	passStdin := flag.Bool("password-stdin", false, "Read password from stdin")
	socketXID := flag.Uint64("socket-xid", 0, "X11 Window ID to embed via GtkPlug")
	scale := flag.Bool("scale", true, "Scale remote desktop to window size")
	keepAspect := flag.Bool("keep-aspect", true, "Keep remote desktop aspect ratio when scaling")
	viewOnly := flag.Bool("view-only", false, "View-only mode (ignore mouse and keyboard input)")
	shared := flag.Bool("shared", false, "Share desktop with other clients")
	title := flag.String("title", "PAC VNC Session", "Window title")
	flag.Parse()

	pass := *password
	if *passStdin {
		raw, err := io.ReadAll(os.Stdin)
		if err == nil {
			pass = strings.TrimRight(string(raw), "\r\n")
		}
	}

	cHost := C.CString(*host)
	defer C.free(unsafe.Pointer(cHost))

	var cUser *C.char
	if *username != "" {
		cUser = C.CString(*username)
		defer C.free(unsafe.Pointer(cUser))
	}

	var cPass *C.char
	if pass != "" {
		cPass = C.CString(pass)
		defer C.free(unsafe.Pointer(cPass))
	}

	cTitle := C.CString(*title)
	defer C.free(unsafe.Pointer(cTitle))

	scaleInt := 0
	if *scale {
		scaleInt = 1
	}
	aspectInt := 0
	if *keepAspect {
		aspectInt = 1
	}
	viewInt := 0
	if *viewOnly {
		viewInt = 1
	}
	sharedInt := 0
	if *shared {
		sharedInt = 1
	}

	ret := C.run_vnc(cHost, C.int(*port), cUser, cPass, C.ulong(*socketXID),
		C.int(scaleInt), C.int(aspectInt), C.int(viewInt), C.int(sharedInt), cTitle)

	if ret != 0 {
		fmt.Fprintf(os.Stderr, "pac-vnc exited with error code %d\n", ret)
		os.Exit(int(ret))
	}
}
