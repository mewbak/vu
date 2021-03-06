// Copyright © 2014-2018 Galvanized Logic Inc.
// Use is governed by a BSD-style license found in the LICENSE file.

package vu

// culler.go encapsulates engine support for culling models prior to render.
// FUTURE: lots of potential culling depends on transform hierarchy work,
//         ie: quad-tree's or other spacial organization schemes.

import (
	"github.com/gazed/vu/math/lin"
)

// Culler reduces the number of items sent for rendering.
// It is attached to a Camera.
type Culler interface {

	// Culled returns true if a model, represented by the Pov p,
	// should be culled using the given camera c.
	Culled(c *Camera, wx, wy, wz float64) bool
}

// =============================================================================

// NewFrontCull returns a culler that keeps objects in a radius directly
// in front of the camera. Objects behind the camera and far away from
// the camera are culled.
func NewFrontCull(r float64) Culler {
	if r < 0 {
		r = 0
	}
	return &frontCull{radius: r, rr: r * r}
}

// frontCull removes everything that is not in a circular radius
// projected in front of the camera.
type frontCull struct {
	radius float64 // regular radius.
	rr     float64 // radius squared.
}

// Culler implmentation.
// Project the part location back along the lookat vector.
func (fc *frontCull) Culled(cam *Camera, wx, wy, wz float64) bool {
	fudgeFactor := float64(0.8) // don't move all the way.
	cx, cy, cz := lin.MultSQ(0, 0, -fc.radius*fudgeFactor, cam.at.Rot)
	wx, wy, wz = wx-cx, wy-cy, wz-cz // move part location back.
	toc := cam.Distance(wx, wy, wz)  // cull if outside radius.
	return toc > fc.rr
}

// =============================================================================

// NewRadiusCull returns a culler that removes objects outside a given
// radius from the camera. Can be used to show objects around a camera,
// eg: top down minimaps.
func NewRadiusCull(r float64) Culler {
	if r < 0 {
		r = 0
	}
	return &radiusCull{rr: r * r}
}

// radiusCull removes everything far away from the camera.
type radiusCull struct {
	rr float64 // radius squared.
}

// Culler implmentation. True if the given location is
// within the culler radius of the camera.
func (rc *radiusCull) Culled(cam *Camera, wx, wy, wz float64) bool {
	return cam.Distance(wx, wy, wz) > rc.rr
}
