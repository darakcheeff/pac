package sftp

import (
	"errors"
	"io"
	"net"
	"testing"

	"github.com/pkg/sftp"
)

func TestIsConnectionLostErr(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{sftp.ErrSSHFxConnectionLost, true},
		{io.EOF, true},
		{io.ErrUnexpectedEOF, true},
		{net.ErrClosed, true},
		{errors.New("connection lost"), true},
		{errors.New("write: broken pipe"), true},
		{errors.New("read: connection reset by peer"), true},
		{errors.New("sftp client is closed"), true},
		{errors.New("permission denied"), false},
		{errors.New("file does not exist"), false},
	}

	for _, tc := range cases {
		if got := isConnectionLostErr(tc.err); got != tc.want {
			t.Errorf("isConnectionLostErr(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}
