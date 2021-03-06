// Copyright © 2013-2018 Galvanized Logic Inc.
// Use is governed by a BSD-style license found in the LICENSE file.

package render

import (
	"fmt"
	"image"
	"log"
	"strings"

	"github.com/gazed/vu/render/gl"
)

// opengl is the OpenGL implementation of Renderer. See the Renderer interface
// for comments. See the OpenGL documentation for OpenGL methods and constants.
type opengl struct {
	depthTest bool   // Track current depth setting to reduce state switching.
	shader    uint32 // Track the current shader to reduce shader switching.
	fbo       uint32 // Track current framebuffer object to reduce switching.
	vw, vh    int32  // Remember the viewport size for framebuffer switching.
}

// newRenderer returns an OpenGL Context.
func newRenderer() Context { return &opengl{} }

// Renderer implementation specific constants.
const (
	// Values useed in Renderer.Enable() method.
	Blend     uint32 = gl.BLEND              // Alpha blending.
	CullFace         = gl.CULL_FACE          // Backface culling.
	DepthTest        = gl.DEPTH_TEST         // Z-buffer (depth) awareness.
	PointSize        = gl.PROGRAM_POINT_SIZE // Enable gl_PointSize in shaders.

	// Vertex data render hints. Used in the Buffer.SetUsage() method.
	StaticDraw  = gl.STATIC_DRAW  // Data created once and rendered many times.
	DynamicDraw = gl.DYNAMIC_DRAW // Data is continually being updated.
)

// Renderer implementation.
func (gc *opengl) Init() error {
	gl.Init()
	return gc.validate()
}

// Renderer implementation.
func (gc *opengl) Color(r, g, b, a float32) { gl.ClearColor(r, g, b, a) }
func (gc *opengl) Clear() {
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
}
func (gc *opengl) Viewport(width int, height int) {
	gc.vw, gc.vh = int32(width), int32(height)
	gl.Viewport(0, 0, int32(width), int32(height))
}

// Renderer implementation.
func (gc *opengl) Enable(attribute uint32, enabled bool) {
	switch attribute {
	case CullFace, DepthTest:
		if enabled {
			gl.Enable(attribute)
		} else {
			gl.Disable(attribute)
		}
	case Blend:
		if enabled {
			gl.Enable(attribute)

			// Using non pre-multiplied alpha color data so...
			gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
		} else {
			gl.Disable(attribute)
		}
	}
}

// Render implementation.
// FUTURE: all kinds of possible optimizations that would need to be
//         profiled before implementing.
//           • group by vao to avoid switching vao's.
//           • group by texture to avoid switching textures.
//           • use interleaved vertex data.
//           • only rebind uniforms when they have changed.
//           • uniform buffers http://www.opengl.org/wiki/Uniform_Buffer_Object.
//           • ... lots more possibilities... leave your fav here.
func (gc *opengl) Render(d *Draw) {
	if d.Scissor {
		gl.Enable(gl.SCISSOR_TEST)         // Each scissor draw needs to...
		gl.Scissor(d.Sx, d.Sy, d.Sw, d.Sh) // ... define its own area.
	}

	// switch state only if necessary.
	if gc.depthTest != d.Depth {
		if d.Depth {
			gl.Enable(gl.DEPTH_TEST)
		} else {
			gl.Disable(gl.DEPTH_TEST)
		}
		gc.depthTest = d.Depth
	}

	// switch render framebuffer only if necessary. The framebuffer
	// is used to render to a texture associated with a framebuffer.
	if gc.fbo != d.Fbo {
		gl.BindFramebuffer(gl.FRAMEBUFFER, d.Fbo)
		if d.Fbo == 0 {
			gl.Viewport(0, 0, gc.vw, gc.vh)
		} else {
			gl.Clear(gl.DEPTH_BUFFER_BIT)
			gl.Viewport(0, 0, LayerSize, LayerSize) // framebuffer texture.
		}
		gc.fbo = d.Fbo
	}

	// switch shaders only if necessary.
	if gc.shader != d.Shader {
		gl.UseProgram(d.Shader)
		gc.shader = d.Shader
	}

	// Ask the model to bind its provisioned uniforms.
	// FUTURE: only need to bind uniforms that have changed.
	gc.bindUniforms(d)

	// bind the data buffers and render.
	// FUTURE: support instanced for more than triangles.
	gl.BindVertexArray(d.Vao)
	switch d.Mode {
	case Lines:
		gl.PolygonMode(gl.FRONT_AND_BACK, gl.LINE)
		gl.DrawElements(gl.LINES, d.FaceCnt, gl.UNSIGNED_SHORT, 0)
		gl.PolygonMode(gl.FRONT_AND_BACK, gl.FILL)
	case Points:
		gl.Enable(gl.PROGRAM_POINT_SIZE)
		gl.DrawArrays(gl.POINTS, 0, d.VertCnt)
		gl.Disable(gl.PROGRAM_POINT_SIZE)
	case Triangles:
		if d.Instances > 0 {
			gl.DrawElementsInstanced(gl.TRIANGLES, d.FaceCnt, gl.UNSIGNED_SHORT, 0, d.Instances)
		} else {
			gl.DrawElements(gl.TRIANGLES, d.FaceCnt, gl.UNSIGNED_SHORT, 0)
		}
	}
	gl.BindVertexArray(0)
	if d.Scissor {
		gl.Disable(gl.SCISSOR_TEST)
	}
}

// bindUniforms links model data to the uniforms discovered
// in the model shader.
// FUTURE: create design that incorporates special cases like
//         textures and animation bone poses.
func (gc *opengl) bindUniforms(d *Draw) {
	for key, ref := range d.Uniforms { // Uniforms expected by the shader.
		switch {
		case key == "bpos":
			if d.Poses != nil && len(d.Poses) > 0 {
				gl.UniformMatrix3x4fv(ref, int32(d.NumPoses), false, &(d.Poses[0]))
			} else {
				log.Printf("Animation data expected for %d", d.Tag)
			}
		case key == "sm":
			gl.Uniform1i(ref, 15) // Convention for shadow maps.
			gl.ActiveTexture(gl.TEXTURE0 + 15)
			gl.BindTexture(gl.TEXTURE_2D, d.Shtex)
		case strings.HasPrefix(key, "uv"):
			// Map the texture order to the texture variable names.
			index := 0
			fmt.Sscanf(key, "uv%d", &index)
			for _, t := range d.Texs {
				if t.order == index {
					gl.Uniform1i(ref, int32(t.order))
					gl.ActiveTexture(gl.TEXTURE0 + uint32(t.order))
					gl.BindTexture(gl.TEXTURE_2D, t.tid)
					break
				}
			}
		default:
			// Everything else is uniform float or matrix data.
			data, ok := d.UniformData[ref] // Uniform data set by engine and App.
			if !ok {
				log.Printf("No uniform data for %d %s", d.Tag, key)
				continue
			}
			switch len(data) {
			case 1:
				gl.Uniform1f(ref, data[0])
			case 2:
				gl.Uniform2f(ref, data[0], data[1])
			case 3:
				gl.Uniform3f(ref, data[0], data[1], data[2])
			case 4:
				gl.Uniform4f(ref, data[0], data[1], data[2], data[3])
			case 16:
				// 4x4 matrix.
				gl.UniformMatrix4fv(ref, 1, false, &(data[0]))
			default:
				log.Printf("Failed to bind %d %s %d", d.Tag, key, len(data))
			}
		}
		if glerr := gl.GetError(); glerr != gl.NO_ERROR {
			log.Printf("bindUniforms %d failed %X %d %s", ref, d.Tag, glerr, key)
		}
	}
}

// validate that OpenGL is available at the right version.
// For OpenGL 3.2 the following lines should be in the report.
//	       [+] glFramebufferTexture
//	       [+] glGetBufferParameteri64v
//	       [+] glGetInteger64i_v
func (gc *opengl) validate() error {
	if report := gl.BindingReport(); len(report) > 0 {
		valid := false
		want := "[+] glFramebufferTexture"
		for _, line := range report {
			if strings.Contains(line, want) {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("Need OpenGL 3.2 or higher")
		}
	} else {
		return fmt.Errorf("OpenGL unavailable")
	}
	return nil
}

// Renderer implementation.
// BindMesh copies the given mesh data to the GPU
// and initializes the vao and buffer references.
func (gc *opengl) BindMesh(vao *uint32, vdata map[uint32]Data, fdata Data) error {
	if glerr := gl.GetError(); glerr != gl.NO_ERROR {
		return fmt.Errorf("BindMesh: fix prior error %X", glerr)
	}

	// Reuse existing vao's.
	if *vao == 0 {
		gl.GenVertexArrays(1, vao)
	}
	gl.BindVertexArray(*vao)
	for _, vbuff := range vdata {
		vd, ok := vbuff.(*vertexData)
		if ok && vd.rebind {
			gc.bindVertexBuffer(vd)
			vd.rebind = false
		}
	}
	if glerr := gl.GetError(); glerr != gl.NO_ERROR {
		return fmt.Errorf("BindMesh failed to bind vb %X", glerr)
	}
	if fd, ok := fdata.(*faceData); ok {
		if fd.rebind {
			gc.bindFaceBuffer(fd)
			fd.rebind = false
		}
	}
	if glerr := gl.GetError(); glerr != gl.NO_ERROR {
		return fmt.Errorf("BindMesh failed to bind fb %X", glerr)
	}
	gl.BindVertexArray(0)
	return nil
}

// bindVertexBuffer copies per-vertex data from the CPU to the GPU.
// The vao is needed only for instanced meshes.
func (gc *opengl) bindVertexBuffer(vdata Data) {
	vd, ok := vdata.(*vertexData)
	if !ok {
		return
	}
	if vd.ref == 0 {
		gl.GenBuffers(1, &vd.ref)
	}
	bytes := 4 // 4 bytes for float32 (gl.FLOAT)
	switch vd.usage {
	case StaticDraw:
		switch {
		case len(vd.floats) > 0:
			gl.BindBuffer(gl.ARRAY_BUFFER, vd.ref)
			gl.BufferData(gl.ARRAY_BUFFER, int64(len(vd.floats)*bytes), gl.Pointer(&(vd.floats[0])), vd.usage)
			gl.VertexAttribPointer(vd.lloc, vd.span, gl.FLOAT, false, 0, 0)
			if vd.instanced {
				// Instanced data are 4x4 transform matricies of floats.
				rowBytes := uint32(4 * 4)       // 4 floats of 4 bytes each per row.
				matrixBytes := int32(4 * 4 * 4) // 4 rows of 4 floats of 4 bytes.
				for cnt := uint32(0); cnt < 4; cnt++ {
					gl.EnableVertexAttribArray(vd.lloc + cnt)
					gl.VertexAttribPointer(vd.lloc+cnt, 4, gl.FLOAT, false, matrixBytes, int64(cnt*rowBytes))

					// Ensure the matrix values are the same for each vertex and
					// will only be updated once the instance changes.
					gl.VertexAttribDivisor(vd.lloc+cnt, 1)
				}
			} else {
				gl.EnableVertexAttribArray(vd.lloc)
			}
		case len(vd.bytes) > 0:
			gl.BindBuffer(gl.ARRAY_BUFFER, vd.ref)
			gl.BufferData(gl.ARRAY_BUFFER, int64(len(vd.bytes)), gl.Pointer(&(vd.bytes[0])), vd.usage)
			gl.VertexAttribPointer(vd.lloc, vd.span, gl.UNSIGNED_BYTE, vd.normalize, 0, 0)
			gl.EnableVertexAttribArray(vd.lloc)
		}
	case DynamicDraw:
		var null gl.Pointer // zero.
		switch {
		case len(vd.floats) > 0:
			gl.BindBuffer(gl.ARRAY_BUFFER, vd.ref)

			// Buffer orphaning, a common way to improve streaming perf. See:
			//         http://www.opengl.org/wiki/Buffer_Object_Streaming
			gl.BufferData(gl.ARRAY_BUFFER, int64(cap(vd.floats)*bytes), null, vd.usage)
			gl.BufferSubData(gl.ARRAY_BUFFER, 0, int64(len(vd.floats)*bytes), gl.Pointer(&(vd.floats[0])))
			gl.VertexAttribPointer(vd.lloc, vd.span, gl.FLOAT, false, 0, 0)
			gl.EnableVertexAttribArray(vd.lloc)
		}
	}
	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
}

// bindFaceBuffer copies triangle face data from the CPU to the GPU.
func (gc *opengl) bindFaceBuffer(fdata Data) {
	fd := fdata.(*faceData)
	if len(fd.data) > 0 {
		if fd.ref == 0 {
			gl.GenBuffers(1, &fd.ref)
		}
		gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, fd.ref)
		bytes := 2 // 2 bytes for uint16 (gl.UNSIGNED_SHORT)
		gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, int64(len(fd.data)*bytes), gl.Pointer(&(fd.data[0])), fd.usage)
	}
}

// Renderer implementation.
// BindShader compiles the shader and makes it available to the GPU.
// It also adds the list of uniforms and vertex layout references to the
// provided maps.
func (gc *opengl) BindShader(vsh, fsh []string, uniforms map[string]int32,
	layouts map[string]uint32) (program uint32, err error) {
	program = gl.CreateProgram()

	// compile and link the shader program.
	if glerr := gl.BindProgram(program, vsh, fsh); glerr != nil {
		err = fmt.Errorf("Failed to create shader program: %s", glerr)
		return
	}

	// initialize the uniform and layout references
	gl.Uniforms(program, uniforms)
	gl.Layouts(program, layouts)
	if glerr := gl.GetError(); glerr != gl.NO_ERROR {
		log.Printf("shader:Bind need to find and fix error %X", glerr)
	}

	// // Deubugging code: dumps shader uniforms once on startup.
	// log.Printf("BindShader")
	// for k, v := range uniforms {
	// 	log.Printf("  uniform %s %d", k, v)
	// }
	return program, err
}

// Renderer implementation.
// BindTexture makes the texture available on the GPU.
func (gc *opengl) BindTexture(tid *uint32, img image.Image) (err error) {
	if glerr := gl.GetError(); glerr != gl.NO_ERROR {
		log.Printf("opengl:bindTexture find and fix prior error %X", glerr)
	}
	if *tid == 0 {
		gl.GenTextures(1, tid)
	}
	gl.BindTexture(gl.TEXTURE_2D, *tid)

	// FUTURE: check if RGBA, or NRGBA are alpha pre-multiplied. The docs say
	// yes for RGBA but the data is from PNG files which are not pre-multiplied
	// and the go png Decode looks like its reading values directly.
	var ptr gl.Pointer
	bounds := img.Bounds()
	width, height := int32(bounds.Dx()), int32(bounds.Dy())
	switch imgType := img.(type) {
	case *image.RGBA:
		i := img.(*image.RGBA)
		ptr = gl.Pointer(&(i.Pix[0]))
	case *image.NRGBA:
		i := img.(*image.NRGBA)
		ptr = gl.Pointer(&(i.Pix[0]))
	default:
		return fmt.Errorf("Unsupported image format %T", imgType)
	}
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, width, height, 0, gl.RGBA, gl.UNSIGNED_BYTE, ptr)
	gl.GenerateMipmap(gl.TEXTURE_2D)
	gc.SetTextureMode(*tid, false) // no repeat by default.
	if glerr := gl.GetError(); glerr != gl.NO_ERROR {
		err = fmt.Errorf("Failed binding texture %d\n", glerr)
	}
	return err
}

// SetTextureMode is used to switch to a clamped
// texture instead of a repeating texture.
func (gc *opengl) SetTextureMode(tid uint32, clamp bool) {
	gl.BindTexture(gl.TEXTURE_2D, tid)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAX_LEVEL, 7)
	if clamp {
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	} else {
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.REPEAT)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.REPEAT)
	}
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST_MIPMAP_LINEAR)
}

// BindTarget creates a framebuffer object that can be used as render
// target. The buffer has both color and depth.
//    http://www.opengl-tutorial.org/intermediate-tutorials/tutorial-14-render-to-texture
func (gc *opengl) BindTarget(fbo, tid, db *uint32) (err error) {
	size := int32(LayerSize)
	gl.GenFramebuffers(1, fbo)
	gl.BindFramebuffer(gl.FRAMEBUFFER, *fbo)

	// Create a texture specifically for the framebuffer.
	gl.GenTextures(1, tid)
	gl.BindTexture(gl.TEXTURE_2D, *tid)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, size, size,
		0, gl.RGBA, gl.UNSIGNED_BYTE, gl.Pointer(nil))
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)

	// Add a depth buffer to mimic the normal framebuffer behaviour for 3D objects.
	gl.GenRenderbuffers(1, db)
	gl.BindRenderbuffer(gl.RENDERBUFFER, *db)
	gl.RenderbufferStorage(gl.RENDERBUFFER, gl.DEPTH_COMPONENT, size, size)
	gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.RENDERBUFFER, *db)

	// Associate the texture with the framebuffer.
	gl.FramebufferTexture(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, *tid, 0)
	buffType := uint32(gl.COLOR_ATTACHMENT0)
	gl.DrawBuffers(1, &buffType)

	// Report any problems.
	glerr := gl.CheckFramebufferStatus(gl.FRAMEBUFFER)
	if glerr != gl.FRAMEBUFFER_COMPLETE {
		return fmt.Errorf("BindFrame error %X", glerr)
	}
	if glerr := gl.GetError(); glerr != gl.NO_ERROR {
		err = fmt.Errorf("Failed binding framebuffer %X", glerr)
	}
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0) // clean up by resetting to default framebuffer.
	return err
}

// BindMap creates a framebuffer object with an associated texture.
// This has depth, but no color. Expected to be used for shadow maps.
//    http://www.opengl-tutorial.org/intermediate-tutorials/tutorial-16-shadow-mapping/
func (gc *opengl) BindMap(fbo, tid *uint32) (err error) {
	size := int32(LayerSize)
	gl.GenFramebuffers(1, fbo)
	gl.BindFramebuffer(gl.FRAMEBUFFER, *fbo)

	// Create a texture specifically for the framebuffer.
	gl.GenTextures(1, tid)
	gl.BindTexture(gl.TEXTURE_2D, *tid)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.DEPTH_COMPONENT16, size, size,
		0, gl.DEPTH_COMPONENT, gl.FLOAT, gl.Pointer(nil))
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_COMPARE_FUNC, gl.LEQUAL)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_COMPARE_MODE, gl.COMPARE_REF_TO_TEXTURE)

	// Associate the texture with the framebuffer.
	gl.FramebufferTexture(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, *tid, 0)
	gl.DrawBuffer(gl.NONE)

	// Report any problems.
	glerr := gl.CheckFramebufferStatus(gl.FRAMEBUFFER)
	if glerr != gl.FRAMEBUFFER_COMPLETE {
		return fmt.Errorf("BindFrame error %X", glerr)
	}
	if glerr := gl.GetError(); glerr != gl.NO_ERROR {
		err = fmt.Errorf("Failed binding framebuffer %X", glerr)
	}
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0) // clean up by resetting to default framebuffer.
	return err
}

// Remove graphic resources.
func (gc *opengl) ReleaseMesh(vao uint32)    { gl.DeleteVertexArrays(1, &vao) }
func (gc *opengl) ReleaseShader(sid uint32)  { gl.DeleteProgram(sid) }
func (gc *opengl) ReleaseTexture(tid uint32) { gl.DeleteTextures(1, &tid) }
func (gc *opengl) ReleaseTarget(fbo, tid, db uint32) {
	gl.DeleteFramebuffers(1, &fbo)
	gl.DeleteTextures(1, &tid)
	gl.DeleteRenderbuffers(1, &db)
}
func (gc *opengl) ReleaseMap(fbo, tid uint32) {
	gl.DeleteFramebuffers(1, &fbo)
	gl.DeleteTextures(1, &tid)
}
