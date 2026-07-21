package main

import (
	"context"
	"io"

	"github.com/davidbyttow/govips/v2/vips"
)

type imageOptions struct {
	maxDimensions *int
	greyscale     bool
}

func transcodeImage(ctx context.Context, src io.Reader, opts *imageOptions) io.Reader {
	pr, pw := io.Pipe()

	stop := context.AfterFunc(ctx, func() {
		pw.CloseWithError(ctx.Err())
	})

	go func() {
		defer stop()

		if err := ctx.Err(); err != nil {
			pw.CloseWithError(err)
			return
		}

		img, err := vips.LoadImageFromReader(src, nil)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		defer img.Close()

		if opts == nil {
			opts = &imageOptions{}
		}

		img, err = newImagePipeline().
			Add(opts.maxDimensions != nil, EnsureMaxDimensions(*opts.maxDimensions)).
			Add(opts.greyscale, Greyscale()).
			Run(img)
		if err != nil {
			pw.CloseWithError(err)
			return
		}

		err = img.SaveToWriter(pw, vips.ImageTypeJPEG, &vips.ExportParams{
			Quality:  50,
			Lossless: false,
		})
		if err != nil {
			pw.CloseWithError(err)
			return
		}

		pw.Close()
	}()

	return pr
}

// Image pipeline
type (
	transformFunc func(img *vips.ImageRef) (*vips.ImageRef, error)
	ImagePipeline []transformFunc
)

func newImagePipeline() *ImagePipeline {
	return &ImagePipeline{}
}

func (p *ImagePipeline) Add(cond bool, tf transformFunc) *ImagePipeline {
	if cond {
		*p = append(*p, tf)
	}
	return p
}

func (p ImagePipeline) Run(img *vips.ImageRef) (*vips.ImageRef, error) {
	var (
		err     error
		current = img
	)

	for _, transform := range p {
		current, err = transform(current)
		if err != nil {
			return nil, err
		}
	}

	return current, nil
}

func EnsureMaxDimensions(maxDim int) transformFunc {
	return func(img *vips.ImageRef) (*vips.ImageRef, error) {
		if img.Width() > maxDim || img.Height() > maxDim {
			err := img.Thumbnail(maxDim, maxDim, vips.InterestingAll)
			return img, err
		}
		return img, nil
	}
}

func Greyscale() transformFunc {
	return func(img *vips.ImageRef) (*vips.ImageRef, error) {
		err := img.ToColorSpace(vips.InterpretationBW)
		return img, err
	}
}
