package fxpakpro

import (
	"context"
	"io"
	"sni/snes"
)

func (d *Device) ReadDirectory(ctx context.Context, path string) ([]snes.DirEntry, error) {
	return d.listFiles(ctx, path)
}

func (d *Device) MakeDirectory(ctx context.Context, path string) error {
	return d.mkdir(ctx, path)
}

func (d *Device) RemoveFile(ctx context.Context, path string) error {
	return d.rm(ctx, path)
}

func (d *Device) RenameFile(ctx context.Context, path, newFilename string) error {
	return d.mv(ctx, path, newFilename)
}

func (d *Device) PutFile(ctx context.Context, path string, size uint32, r io.Reader, progress snes.ProgressReportFunc) (n uint32, err error) {
	n, err = d.putFile(ctx, path, size, r, progress)
	return
}

func (d *Device) GetFile(ctx context.Context, path string, w io.Writer, sizeReceived snes.SizeReceivedFunc, progress snes.ProgressReportFunc) (size uint32, err error) {
	size, err = d.getFile(ctx, path, w, sizeReceived, progress)
	return
}

func (d *Device) BootFile(ctx context.Context, path string) error {
	return d.boot(ctx, path)
}
