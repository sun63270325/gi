// Copyright 2019 The GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// based on golang.org/x/exp/shiny:
// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package glos

import (
	"image"
	"image/color"
	"image/draw"
	"log"

	"github.com/goki/gi/mat32"
	"github.com/goki/gi/oswin"
	"github.com/goki/gi/oswin/gpu"
)

func (app *appImpl) initDrawProgs() error {
	if app.progInit {
		return nil
	}
	p := theGPU.NewProgram("draw")
	_, err := p.AddShader(gpu.VertexShader, "draw-vert", drawVertSrc)
	if err != nil {
		return err
	}
	_, err = p.AddShader(gpu.FragmentShader, "draw-frag", drawFragSrc)
	if err != nil {
		return err
	}
	p.AddUniform("mvp", gpu.UniType{Type: gpu.Float32, Mat: 3}, false, 0)
	p.AddUniform("uvp", gpu.UniType{Type: gpu.Float32, Mat: 3}, false, 0)
	p.AddUniform("sample", gpu.UniType{Type: gpu.Int}, false, 0)

	pv := p.AddInput("pos", gpu.VectorType{Type: gpu.Float32, Vec: 2}, gpu.VertexPosition)

	p.SetFragDataVar("outputColor")

	err = p.Compile()
	if err != nil {
		return err
	}
	app.drawProg = p
	gpu.TheGPU.ErrCheck("initDrawProgs -- draw compile")

	b := theGPU.NewBufferMgr()
	vb := b.AddVectorsBuffer(gpu.StaticDraw)
	vb.AddVectors(pv, false)
	vb.SetLen(len(quadCoords))
	vb.SetAllData(quadCoords)
	b.Activate()
	gpu.TheGPU.ErrCheck("initDrawProgs -- b activate")
	app.drawQuads = b

	p = theGPU.NewProgram("fill")
	_, err = p.AddShader(gpu.VertexShader, "fill-vert", fillVertSrc)
	if err != nil {
		return err
	}
	_, err = p.AddShader(gpu.FragmentShader, "fill-frag", fillFragSrc)
	if err != nil {
		return err
	}
	p.AddUniform("mvp", gpu.UniType{Type: gpu.Float32, Mat: 3}, false, 0)
	p.AddUniform("color", gpu.UniType{Type: gpu.Float32, Vec: 4}, false, 0)

	p.AddInput("pos", gpu.VectorType{Type: gpu.Float32, Vec: 2}, gpu.VertexPosition)

	p.SetFragDataVar("outputColor")

	err = p.Compile()
	if err != nil {
		return err
	}
	app.fillProg = p
	gpu.TheGPU.ErrCheck("initDrawProgs -- fill compile")

	b = theGPU.NewBufferMgr()
	vb = b.AddVectorsBuffer(gpu.StaticDraw)
	vb.AddVectors(pv, false)
	vb.SetLen(len(quadCoords))
	vb.SetAllData(quadCoords)
	b.Activate()
	app.fillQuads = b

	err = gpu.TheGPU.ErrCheck("initDrawProgs")
	if err != nil {
		return err
	}
	app.progInit = true
	return nil
}

// draw draws to current render target (could be window or framebuffer / texture)
// proper context must have already been established outside this call!
func (app *appImpl) draw(dstSz image.Point, src2dst mat32.Matrix3, src oswin.Texture, sr image.Rectangle, op draw.Op, opts *oswin.DrawOptions) {

	t := src.(*textureImpl)
	sr = sr.Intersect(t.Bounds())
	if sr.Empty() {
		return
	}

	gpu.Draw.Op(op)
	app.drawProg.Activate()

	// Start with src-space left, top, right and bottom.
	srcL := float32(sr.Min.X)
	srcT := float32(sr.Min.Y)
	srcR := float32(sr.Max.X)
	srcB := float32(sr.Max.Y)

	// Transform to dst-space via the src2dst matrix, then to a MVP matrix.
	matMVP := calcMVP(dstSz.X, dstSz.Y,
		src2dst[0]*srcL+src2dst[3]*srcT+src2dst[6],
		src2dst[1]*srcL+src2dst[4]*srcT+src2dst[7],
		src2dst[0]*srcR+src2dst[3]*srcT+src2dst[6],
		src2dst[1]*srcR+src2dst[4]*srcT+src2dst[7],
		src2dst[0]*srcL+src2dst[3]*srcB+src2dst[6],
		src2dst[1]*srcL+src2dst[4]*srcB+src2dst[7],
	)
	err := app.drawProg.UniformByName("mvp").SetValue(matMVP)
	if err != nil {
		log.Println(err)
	}

	// OpenGL's fragment shaders' UV coordinates run from (0,0)-(1,1),
	// unlike vertex shaders' XY coordinates running from (-1,+1)-(+1,-1).
	//
	// We are drawing a rectangle PQRS, defined by two of its
	// corners, onto the entire texture. The two quads may actually
	// be equal, but in the general case, PQRS can be smaller.
	//
	//	(0,0) +---------------+ (1,0)
	//	      |  P +-----+ Q  |
	//	      |    |     |    |
	//	      |  S +-----+ R  |
	//	(0,1) +---------------+ (1,1)
	//
	// The PQRS quad is always axis-aligned. First of all, convert
	// from pixel space to texture space.
	tw := float32(t.size.X)
	th := float32(t.size.Y)
	px := float32(sr.Min.X-0) / tw
	py := float32(sr.Min.Y-0) / th
	qx := float32(sr.Max.X-0) / tw
	sy := float32(sr.Max.Y-0) / th
	// Due to axis alignment, qy = py and sx = px.
	//
	// The simultaneous equations are:
	//	  0 +   0 + a02 = px
	//	  0 +   0 + a12 = py
	//	a00 +   0 + a02 = qx
	//	a10 +   0 + a12 = qy = py
	//	  0 + a01 + a02 = sx = px
	//	  0 + a11 + a12 = sy
	matUVP := mat32.Matrix3{
		qx - px, 0,
		0, sy - py,
		px, py,
	}
	err = app.drawProg.UniformByName("uvp").SetValue(matUVP)
	if err != nil {
		log.Println(err)
	}
	gpu.TheGPU.ErrCheck("draw -- uvp")

	t.Activate(0)
	err = app.drawProg.UniformByName("sample").SetValue(int32(0))
	if err != nil {
		log.Println(err)
	}
	gpu.TheGPU.ErrCheck("draw -- sample")

	app.drawQuads.Activate()
	gpu.Draw.TriangleStrips(0, 4)
}

// fill fills to current render target (could be window or framebuffer / texture)
// proper context must have already been established outside this call!
func (app *appImpl) fill(mvp mat32.Matrix3, src color.Color, op draw.Op) {
	gpu.Draw.Op(op)
	app.fillProg.Activate()

	app.fillProg.UniformByName("mvp").SetValue(mvp)

	r, g, b, a := src.RGBA()

	clvec4 := mat32.NewVector4(
		float32(r)/65535,
		float32(g)/65535,
		float32(b)/65535,
		float32(a)/65535,
	)

	app.fillProg.UniformByName("color").SetValue(clvec4)

	app.fillQuads.Activate()
	gpu.Draw.TriangleStrips(0, 4)
}

// fillRect fills given rectangle, where dstSz is overall size of the destination (e.g., window)
func (app *appImpl) fillRect(dstSz image.Point, dr image.Rectangle, src color.Color, op draw.Op) {
	minX := float32(dr.Min.X)
	minY := float32(dr.Min.Y)
	maxX := float32(dr.Max.X)
	maxY := float32(dr.Max.Y)

	mvp := calcMVP(dstSz.X, dstSz.Y,
		minX, minY,
		maxX, minY,
		minX, maxY,
	)
	app.fill(mvp, src, op)
}

// drawUniform does a fill-like uniform color fill but with an arbitrary src2dst transform
func (app *appImpl) drawUniform(dstSz image.Point, src2dst mat32.Matrix3, src color.Color, sr image.Rectangle, op draw.Op, opts *oswin.DrawOptions) {
	minX := float32(sr.Min.X)
	minY := float32(sr.Min.Y)
	maxX := float32(sr.Max.X)
	maxY := float32(sr.Max.Y)

	// Transform to dst-space via the src2dst matrix, then to a MVP matrix.
	mvp := calcMVP(dstSz.X, dstSz.Y,
		src2dst[0]*minX+src2dst[3]*minY+src2dst[6],
		src2dst[1]*minX+src2dst[4]*minY+src2dst[7],
		src2dst[0]*maxX+src2dst[3]*minY+src2dst[6],
		src2dst[1]*maxX+src2dst[4]*minY+src2dst[7],
		src2dst[0]*minX+src2dst[3]*maxY+src2dst[6],
		src2dst[1]*minX+src2dst[4]*maxY+src2dst[7],
	)
	app.fill(mvp, src, op)
}

// calcMVP returns the Model View Projection matrix that maps the quadCoords
// unit square, (0, 0) to (1, 1), to a quad QV, such that QV in vertex shader
// space corresponds to the quad QP in pixel space, where QP is defined by
// three of its four corners - the arguments to this function. The three
// corners are nominally the top-left, top-right and bottom-left, but there is
// no constraint that e.g. tlx < trx.
//
// In pixel space, the window ranges from (0, 0) to (widthPx, heightPx). The
// Y-axis points downwards.
//
// In vertex shader space, the window ranges from (-1, +1) to (+1, -1), which
// is a 2-unit by 2-unit square. The Y-axis points upwards.
func calcMVP(widthPx, heightPx int, tlx, tly, trx, try, blx, bly float32) mat32.Matrix3 {
	// Convert from pixel coords to vertex shader coords.
	invHalfWidth := +2 / float32(widthPx)
	invHalfHeight := -2 / float32(heightPx)
	tlx = tlx*invHalfWidth - 1
	tly = tly*invHalfHeight + 1
	trx = trx*invHalfWidth - 1
	try = try*invHalfHeight + 1
	blx = blx*invHalfWidth - 1
	bly = bly*invHalfHeight + 1

	// The resultant affine matrix:
	//	- maps (0, 0) to (tlx, tly).
	//	- maps (1, 0) to (trx, try).
	//	- maps (0, 1) to (blx, bly).
	return mat32.Matrix3{
		trx - tlx, try - tly,
		blx - tlx, bly - tly,
		tlx, tly,
	}
}

var quadCoords = mat32.ArrayF32{
	0, 0, // top left
	1, 0, // top right
	0, 1, // bottom left
	1, 1, // bottom right
}

const drawVertSrc = `
#version 330

uniform mat3 mvp;
uniform mat3 uvp;

in vec2 pos;

out vec2 uv;

void main() {
	vec3 p = vec3(pos, 1);
	gl_Position = vec4(mvp * p, 1);
	uv = (uvp * vec3(pos, 1)).xy;
}
` + "\x00"

const drawFragSrc = `
#version 330

precision mediump float;

uniform sampler2D sample;

in vec2 uv;

out vec4 outputColor;

void main() {
	outputColor = texture(sample, uv);
}
` + "\x00"

const fillVertSrc = `
#version 330

uniform mat3 mvp;

in vec2 pos;

void main() {
	vec3 p = vec3(pos, 1);
	gl_Position = vec4(mvp * p, 1);
}
` + "\x00"

const fillFragSrc = `
#version 330

precision mediump float;

uniform vec4 color;

out vec4 outputColor;

void main() {
	outputColor = color;
}
` + "\x00"
