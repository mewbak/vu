// Copyright © 2014-2018 Galvanized Logic Inc.
// Use is governed by a BSD-style license found in the LICENSE file.

package main

import (
	"log"
	"math"
	"math/rand"
	"time"

	"github.com/gazed/vu"
	"github.com/gazed/vu/ai"
	"github.com/gazed/vu/grid"
)

// ff demonstrates flow field pathing by having a bunch of chasers and a goal.
// The goal is randomly reset once all the chasers have reached it.
// Restarting the example will create a different random grid.
// See vu/grid/flow.go for more information on flow fields.
//
// CONTROLS:
//   Sp    : pause while pressed
func ff() {
	defer catchErrors()
	if err := vu.Run(&fftag{}); err != nil {
		log.Printf("ff: error starting engine %s", err)
	}
}

// Globally unique "tag" that encapsulates example specific data.
type fftag struct {
	ui        *vu.Ent   // transform hierarchy root.
	chaseRoot *vu.Ent   // parent of chaser instances.
	chasers   []*chaser // map chasers.
	goal      *vu.Ent   // chasers goal.
	mmap      *vu.Ent   // allows the main map to be moved around.
	msize     int       // map width and height.
	spots     []int     // unique ids of open spots.
	plan      grid.Grid // the floor layout.
	flow      ai.Flow   // the flow field.
}

// Create is the engine callback for initial asset creation.
func (ff *fftag) Create(eng vu.Eng, s *vu.State) {
	eng.Set(vu.Title("Flow Field"), vu.Size(400, 100, 750, 750))
	eng.Set(vu.Color(0.15, 0.15, 0.15, 1))
	rand.Seed(time.Now().UTC().UnixNano())

	// create the 2D overlay
	ff.ui = eng.AddScene().SetUI()
	ff.ui.Cam().SetClip(0, 10)
	ff.mmap = ff.ui.AddPart().SetScale(10, 10, 0)
	ff.mmap.SetAt(30, 30, 0)

	// Use model instances to draw all the unmoving blocks.
	block := ff.mmap.AddPart()
	block.MakeInstancedModel("texturedInstanced", "msh:icon", "tex:wall")

	// populate the map
	ff.msize = 69
	ff.plan = grid.New(grid.RoomSkirmish)
	ff.plan.Generate(ff.msize, ff.msize)
	width, height := ff.plan.Size()
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			if ff.plan.IsOpen(x, y) {
				// open spots used to navigate.
				ff.spots = append(ff.spots, ff.id(x, y))
			} else {
				// less resources used showing walls rather than open spots.
				block.AddPart().SetAt(float64(x), float64(y), 0)
			}
		}
	}

	// populate chasers and a goal.
	numChasers := 30
	ff.chaseRoot = ff.mmap.AddPart()
	ff.chaseRoot.MakeInstancedModel("texturedInstanced", "msh:icon", "tex:token")
	for cnt := 0; cnt < numChasers; cnt++ {
		ff.chasers = append(ff.chasers, newChaser(ff.chaseRoot))
	}
	ff.goal = ff.mmap.AddPart().MakeModel("textured", "msh:icon", "tex:goal")
	ff.flow = ai.NewGridFlow(ff.plan) // grid flow field for the given plan.
	ff.resetLocations()
}

// Update is the regular engine callback.
func (ff *fftag) Update(eng vu.Eng, in *vu.Input, s *vu.State) {
	if _, ok := in.Down[vu.KSpace]; ok {
		return // pause with space bar.
	}

	// move each of the chasers closer to the goal.
	reset := true
	for _, chaser := range ff.chasers {
		if chaser.move(ff.flow) {
			reset = false
		}
	}
	if reset {
		ff.resetLocations()
	}
}

// resetLocations randomly distributes the chasers around the map.
func (ff *fftag) resetLocations() {
	for _, chaser := range ff.chasers {
		spot := ff.spots[rand.Intn(len(ff.spots))] // get open location.
		chaser.gx, chaser.gy = ff.at(spot)         // get map location.
		chaser.nx, chaser.ny = chaser.gx, chaser.gy
		chaser.pov.SetAt(float64(chaser.gx), float64(chaser.gy), 0)
	}
	spot := ff.spots[rand.Intn(len(ff.spots))]
	goalx, goaly := ff.at(spot)
	ff.goal.SetAt(float64(goalx), float64(goaly), 0)

	// create the flow field based on the given goal.
	ff.flow.Create(goalx, goaly)
}

// Turn x,y map indicies to unique identifiers.
func (ff *fftag) id(x, y int) int { return x*ff.msize + y }

// Turn unique identifiers to x,y map indicies.
func (ff *fftag) at(id int) (x, y int) { return id / ff.msize, id % ff.msize }

// =============================================================================

// chasers move from grid location to grid location until they
// reach the goal.
type chaser struct {
	pov    *vu.Ent // actual location.
	gx, gy int     // old grid location.
	nx, ny int     // next grid location.
	cx, cy int     // optional center to avoid when moving.
}

// chaser moves towards a goal.
func newChaser(parent *vu.Ent) *chaser {
	c := &chaser{}
	c.pov = parent.AddPart() // new instance.
	return c
}

// move the chaser a bit closer to its goal.
func (c *chaser) move(flow ai.Flow) (moved bool) {
	sx, sy, _ := c.pov.At() // actual screen location.
	atx := math.Abs(float64(sx-float64(c.nx))) < 0.05
	aty := math.Abs(float64(sy-float64(c.ny))) < 0.05
	if atx && aty { // reached next location.
		c.gx, c.gy = c.nx, c.ny
		nx, ny := flow.Next(c.gx, c.gy)
		if nx == 9 {
			return false // no valid moves for this chaser.
		}
		if nx == 0 && ny == 0 {
			return false // reached the goal
		}
		moved = true
		c.nx, c.ny = c.gx+nx, c.gy+ny
		c.pov.SetAt(float64(c.gx), float64(c.gy), 0)

		// check if the chaser path should go around a corner.
		c.cx, c.cy = 0, 0
		switch {
		case nx == -1 && ny == 1:
			if ax, _ := flow.Next(c.gx-1, c.gy); ax == 9 {
				c.cx, c.cy = c.gx-1, c.gy
			}
			if bx, _ := flow.Next(c.gx, c.gy+1); bx == 9 {
				c.cx, c.cy = c.gx, c.gy+1
			}
		case nx == -1 && ny == -1:
			if ax, _ := flow.Next(c.gx-1, c.gy); ax == 9 {
				c.cx, c.cy = c.gx-1, c.gy
			}
			if bx, _ := flow.Next(c.gx, c.gy-1); bx == 9 {
				c.cx, c.cy = c.gx, c.gy-1
			}
		case nx == 1 && ny == 1:
			if ax, _ := flow.Next(c.gx+1, c.gy); ax == 9 {
				c.cx, c.cy = c.gx+1, c.gy
			}
			if bx, _ := flow.Next(c.gx, c.gy+1); bx == 9 {
				c.cx, c.cy = c.gx, c.gy+1
			}
		case nx == 1 && ny == -1:
			if ax, _ := flow.Next(c.gx+1, c.gy); ax == 9 {
				c.cx, c.cy = c.gx+1, c.gy
			}
			if bx, _ := flow.Next(c.gx, c.gy-1); bx == 9 {
				c.cx, c.cy = c.gx, c.gy-1
			}
		}
	} else {
		moved = true
		speed := 0.1 // move a bit closer to the next spot.
		if c.cx == 0 && c.cy == 0 {
			// move in a straight line.
			if !atx {
				sx += float64(c.nx-c.gx) * speed
			}
			if !aty {
				sy += float64(c.ny-c.gy) * speed
			}
			c.pov.SetAt(sx, sy, 0)
		} else {
			// move in a straight line.
			if !atx {
				sx += float64(c.nx-c.gx) * speed
			}
			if !aty {
				sy += float64(c.ny-c.gy) * speed
			}

			// push the point out so that it moves around the
			// circle radius of center cx, cy.
			dx, dy := sx-float64(c.cx), sy-float64(c.cy)
			vlen := math.Sqrt(dx*dx + dy*dy) // vector length.
			if vlen != 0 {
				dx, dy = dx/vlen, dy/vlen // unit vector.
			}
			radius := 1.0
			sx, sy = float64(c.cx)+dx*radius, float64(c.cy)+dy*radius
			c.pov.SetAt(sx, sy, 0)
		}
	}
	return moved
}
