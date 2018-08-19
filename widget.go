// Copyright (c) 2018, The GoKi Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gi

import (
	"fmt"
	"image"
	"log"
	"strings"

	"github.com/goki/gi/oswin"
	"github.com/goki/gi/oswin/mouse"
	"github.com/goki/gi/units"
	"github.com/goki/ki"
	"github.com/goki/ki/bitflag"
	"github.com/goki/ki/kit"
)

// WidgetBase is the base type for all Widget Node2D elements, which are
// managed by a containing Layout, and use all 5 rendering passes.  All
// elemental widgets must support the Node Inactive and Selected states in a
// reasonable way (Selected only essential when also Inactive), so they can
// function appropriately in a chooser (e.g., SliceView or TableView) -- this
// includes toggling selection on left mouse press.
type WidgetBase struct {
	Node2DBase
	Tooltip      string       `desc:"text for tooltip for this widget -- can use HTML formatting"`
	Sty          Style        `json:"-" xml:"-" desc:"styling settings for this widget -- set in SetStyle2D during an initialization step, and when the structure changes"`
	DefStyle     *Style       `view:"-" json:"-" xml:"-" desc:"default style values computed by a parent widget for us -- if set, we are a part of a parent widget and should use these as our starting styles instead of type-based defaults"`
	LayData      LayoutData   `json:"-" xml:"-" desc:"all the layout information for this item"`
	WidgetSig    ki.Signal    `json:"-" xml:"-" view:"-" desc:"general widget signals supported by all widgets, including select, focus, and context menu (right mouse button) events, which can be used by views and other compound widgets"`
	CtxtMenuFunc CtxtMenuFunc `view:"-" json:"-" xml:"-" desc:"optional context menu function called by MakeContextMenu AFTER any native items are added -- this function can decide where to insert new elements -- typically add a separator to disambiguate"`
}

var KiT_WidgetBase = kit.Types.AddType(&WidgetBase{}, WidgetBaseProps)

var WidgetBaseProps = ki.Props{
	"base-type": true,
}

func (g *WidgetBase) AsWidget() *WidgetBase {
	return g
}

// Style satisfies the Styler interface
func (g *WidgetBase) Style() *Style {
	return &g.Sty
}

// Init2DWidget handles basic node initialization -- Init2D can then do special things
func (g *WidgetBase) Init2DWidget() {
	g.Viewport = g.ParentViewport()
	g.Sty.Defaults()
	g.LayData.Defaults() // doesn't overwrite
	g.ConnectToViewport()
}

func (g *WidgetBase) Init2D() {
	g.Init2DWidget()
}

// DefaultStyle2DWidget retrieves default style object for the type, from type
// properties -- selector is optional selector for state etc.  Property key is
// "__DefStyle" + selector -- if part != nil, then use that obj for getting the
// default style starting point when creating a new style
func (g *WidgetBase) DefaultStyle2DWidget(selector string, part *WidgetBase) *Style {
	tprops := kit.Types.Properties(g.Type(), true) // true = makeNew
	styprops := tprops
	if selector != "" {
		sp, ok := tprops[selector]
		if !ok {
			// log.Printf("gi.DefaultStyle2DWidget: did not find props for style selector: %v for node type: %v\n", selector, g.Type().Name())
		} else {
			spm, ok := sp.(ki.Props)
			if !ok {
				log.Printf("gi.DefaultStyle2DWidget: looking for a ki.Props for style selector: %v, instead got type: %T, for node type: %v\n", selector, spm, g.Type().Name())
			} else {
				styprops = spm
			}
		}
	}

	parSty := g.ParentStyle()

	var dsty *Style
	pnm := "__DefStyle" + selector
	dstyi, ok := tprops[pnm]
	if !ok || RebuildDefaultStyles {
		dsty = &Style{}
		dsty.Defaults()
		if selector != "" {
			var baseStyle *Style
			if part != nil {
				baseStyle = part.DefaultStyle2DWidget("", nil)
			} else {
				baseStyle = g.DefaultStyle2DWidget("", nil)
			}
			*dsty = *baseStyle
		}
		dsty.SetStyleProps(parSty, styprops)
		dsty.IsSet = false // keep as non-set
		tprops[pnm] = dsty
	} else {
		dsty, _ = dstyi.(*Style)
	}
	return dsty
}

// Style2DWidget styles the Style values from node properties and optional
// base-level defaults -- for Widget-style nodes
func (g *WidgetBase) Style2DWidget() {
	gii, _ := g.This.(Node2D)
	SetCurStyleNode2D(gii)
	defer SetCurStyleNode2D(nil)
	if !RebuildDefaultStyles && g.DefStyle != nil {
		g.Sty.CopyFrom(g.DefStyle)
	} else {
		g.Sty.CopyFrom(g.DefaultStyle2DWidget("", nil))
	}
	g.Sty.IsSet = false // this is always first call, restart

	if g.Viewport == nil { // robust
		gii.Init2D()
	}

	styprops := *g.Properties()
	parSty := g.ParentStyle()
	g.Sty.SetStyleProps(parSty, styprops)

	// look for class-specific style sheets among defaults -- have to do these
	// dynamically now -- cannot compile into default which is type-general
	tprops := kit.Types.Properties(g.Type(), true) // true = makeNew
	clsty := "." + g.Class
	if sp := ki.SubProps(tprops, clsty); sp != nil {
		g.Sty.SetStyleProps(parSty, sp)
	}

	pagg := g.ParentCSSAgg()
	if pagg != nil {
		AggCSS(&g.CSSAgg, *pagg)
	} else {
		g.CSSAgg = nil // restart
	}
	AggCSS(&g.CSSAgg, g.CSS)
	g.Sty.StyleCSS(gii, g.CSSAgg, "")

	g.Sty.SetUnitContext(g.Viewport, Vec2DZero) // todo: test for use of el-relative
	g.LayData.SetFromStyle(&g.Sty.Layout)       // also does reset
	if g.Sty.Inactive {                         // inactive can only set, not clear
		g.SetInactive()
	}
	g.Sty.Use() // activates currentColor etc
}

// StylePart sets the style properties for a child in parts (or any other
// child) based on its name -- only call this when new parts were created --
// name of properties is #partname (lower cased) and it should contain a
// ki.Props which is then added to the part's props -- this provides built-in
// defaults for parts, so it is separate from the CSS process
func (g *WidgetBase) StylePart(pk Node2D) {
	if pk == nil {
		return
	}
	pg := pk.AsWidget()
	if pg == nil {
		return
	}
	if pg.DefStyle != nil && !RebuildDefaultStyles { // already set
		return
	}
	stynm := "#" + strings.ToLower(pk.Name())
	// this is called on US (the parent object) so we store the #partname
	// default style within our type properties..  that's good -- HOWEVER we
	// cannot put any sub-selector properties within these part styles -- must
	// all be in the base-level.. hopefully that works..
	pdst := g.DefaultStyle2DWidget(stynm, pg)
	pg.DefStyle = pdst // will use this as starting point for all styles now..

	if ics := pk.Embed(KiT_Icon); ics != nil {
		ic := ics.(*Icon)
		styprops := kit.Types.Properties(g.Type(), true)
		sp := ki.SubProps(styprops, stynm)
		if sp != nil {
			if fill, ok := sp["fill"]; ok {
				ic.SetProp("fill", fill)
			}
			if stroke, ok := sp["stroke"]; ok {
				ic.SetProp("stroke", stroke)
			}
		}
		sp = ki.SubProps(*g.Properties(), stynm)
		if sp != nil {
			for k, v := range sp {
				ic.SetProp(k, v)
			}
		}
	}
}

func (g *WidgetBase) Style2D() {
	g.Style2DWidget()
}

func (g *WidgetBase) InitLayout2D() {
	g.LayData.SetFromStyle(&g.Sty.Layout)
}

func (g *WidgetBase) Size2DBase() {
	g.InitLayout2D()
}

func (g *WidgetBase) Size2D() {
	g.Size2DBase()
}

// AddParentPos adds the position of our parent to our layout position --
// layout computations are all relative to parent position, so they are
// finally cached out at this stage also returns the size of the parent for
// setting units context relative to parent objects
func (g *WidgetBase) AddParentPos() Vec2D {
	if pni, _ := KiToNode2D(g.Par); pni != nil {
		if pw := pni.AsWidget(); pw != nil {
			if !g.IsField() {
				g.LayData.AllocPos = pw.LayData.AllocPos.Add(g.LayData.AllocPosRel)
			}
			return pw.LayData.AllocSize
		}
	}
	return Vec2DZero
}

// get our bbox from Layout allocation
func (g *WidgetBase) BBoxFromAlloc() image.Rectangle {
	return RectFromPosSize(g.LayData.AllocPos, g.LayData.AllocSize)
}

func (g *WidgetBase) BBox2D() image.Rectangle {
	return g.BBoxFromAlloc()
}

func (g *WidgetBase) ComputeBBox2D(parBBox image.Rectangle, delta image.Point) {
	g.ComputeBBox2DBase(parBBox, delta)
}

// Layout2DBase provides basic Layout2D functions -- good for most cases
func (g *WidgetBase) Layout2DBase(parBBox image.Rectangle, initStyle bool) {
	nii, _ := g.This.(Node2D)
	if g.Viewport == nil { // robust
		if nii.AsViewport2D() == nil {
			nii.Init2D()
			nii.Style2D()
			// fmt.Printf("node not init in Layout2DBase: %v\n", g.PathUnique())
		}
	}
	psize := g.AddParentPos()
	g.LayData.AllocPosOrig = g.LayData.AllocPos
	if initStyle {
		g.Sty.SetUnitContext(g.Viewport, psize) // update units with final layout
	}
	g.BBox = nii.BBox2D() // only compute once, at this point
	// note: if other styles are maintained, they also need to be updated!
	nii.ComputeBBox2D(parBBox, image.ZP) // other bboxes from BBox
	// typically Layout2DChildren must be called after this!
	if Layout2DTrace {
		fmt.Printf("Layout: %v alloc pos: %v size: %v vpbb: %v winbb: %v\n", g.PathUnique(), g.LayData.AllocPos, g.LayData.AllocSize, g.VpBBox, g.WinBBox)
	}
}

func (g *WidgetBase) Layout2D(parBBox image.Rectangle) {
	g.Layout2DBase(parBBox, true)
	g.Layout2DChildren()
}

// ChildrenBBox2DWidget provides a basic widget box-model subtraction of
// margin and padding to children -- call in ChildrenBBox2D for most widgets
func (g *WidgetBase) ChildrenBBox2DWidget() image.Rectangle {
	nb := g.VpBBox
	spc := int(g.Sty.BoxSpace())
	nb.Min.X += spc
	nb.Min.Y += spc
	nb.Max.X -= spc
	nb.Max.Y -= spc
	return nb
}

func (g *WidgetBase) ChildrenBBox2D() image.Rectangle {
	return g.ChildrenBBox2DWidget()
}

// FullReRenderIfNeeded tests if the FullReRender flag has been set, and if
// so, calls ReRender2DTree and returns true -- call this at start of each
// Render2D
func (g *WidgetBase) FullReRenderIfNeeded() bool {
	if !g.VpBBox.Empty() && g.NeedsFullReRender() {
		if Render2DTrace {
			fmt.Printf("Render: NeedsFullReRender for %v at %v\n", g.PathUnique(), g.VpBBox)
		}
		g.ClearFullReRender()
		g.ReRender2DTree()
		return true
	}
	return false
}

// PushBounds pushes our bounding-box bounds onto the bounds stack if non-empty
// -- this limits our drawing to our own bounding box, automatically -- must
// be called as first step in Render2D returns whether the new bounds are
// empty or not -- if empty then don't render!
func (g *WidgetBase) PushBounds() bool {
	if g.IsOverlay() {
		g.ClearFullReRender()
		if g.Viewport != nil {
			g.ConnectToViewport()
			g.Viewport.Render.PushBounds(g.Viewport.Pixels.Bounds())
		}
		return true
	}
	if g.VpBBox.Empty() {
		g.ClearFullReRender()
		return false
	}
	rs := &g.Viewport.Render
	rs.PushBounds(g.VpBBox)
	g.ConnectToViewport()
	if Render2DTrace {
		fmt.Printf("Render: %v at %v\n", g.PathUnique(), g.VpBBox)
	}
	return true
}

// PopBounds pops our bounding-box bounds -- last step in Render2D after
// rendering children
func (g *WidgetBase) PopBounds() {
	g.ClearFullReRender()
	rs := &g.Viewport.Render
	rs.PopBounds()
}

func (g *WidgetBase) Render2D() {
	if g.FullReRenderIfNeeded() {
		return
	}
	if g.PushBounds() {
		// connect to events here
		g.Render2DChildren()
		g.PopBounds()
	} else {
		g.DisconnectAllEvents(RegPri)
	}
}

// ReRender2DTree does a re-render of the tree -- after it has already been
// initialized and styled -- just does layout and render passes
func (g *WidgetBase) ReRender2DTree() {
	ld := g.LayData // save our current layout data
	updt := g.UpdateStart()
	g.Init2DTree()
	g.Style2DTree()
	g.Size2DTree()
	g.LayData = ld // restore
	g.Layout2DTree()
	g.Render2DTree()
	g.UpdateEndNoSig(updt)
}

// Move2DBase does the basic move on this node
func (g *WidgetBase) Move2DBase(delta image.Point, parBBox image.Rectangle) {
	g.LayData.AllocPos = g.LayData.AllocPosOrig.Add(NewVec2DFmPoint(delta))
	g.This.(Node2D).ComputeBBox2D(parBBox, delta)
}

func (g *WidgetBase) Move2D(delta image.Point, parBBox image.Rectangle) {
	g.Move2DBase(delta, parBBox)
	g.Move2DChildren(delta)
}

// Move2DTree does move2d pass -- each node iterates over children for maximum
// control -- this starts with parent VpBBox and current delta -- can be
// called de novo
func (g *WidgetBase) Move2DTree() {
	if g.HasNoLayout() {
		return
	}
	parBBox := image.ZR
	_, pg := KiToNode2D(g.Par)
	if pg != nil {
		parBBox = pg.VpBBox
	}
	delta := g.LayData.AllocPos.Sub(g.LayData.AllocPosOrig).ToPoint()
	g.This.(Node2D).Move2D(delta, parBBox) // important to use interface version to get interface!
}

// ParentLayout returns the parent layout
func (g *WidgetBase) ParentLayout() *Layout {
	var parLy *Layout
	g.FuncUpParent(0, g.This, func(k ki.Ki, level int, d interface{}) bool {
		nii, ok := k.(Node2D)
		if !ok {
			return false // don't keep going up
		}
		ly := nii.AsLayout2D()
		if ly != nil {
			parLy = ly
			return false // done
		}
		return true
	})
	return parLy
}

// CtxtMenuFunc is a function for creating a context menu for given node
type CtxtMenuFunc func(g Node2D, m *Menu)

func (g *WidgetBase) MakeContextMenu(m *Menu) {
	// derived types put native menu code here
	if g.CtxtMenuFunc != nil {
		g.CtxtMenuFunc(g.This.(Node2D), m)
	}
}

var TooltipFrameProps = ki.Props{
	"border-width":        units.NewValue(0, units.Px),
	"border-color":        "none",
	"margin":              units.NewValue(0, units.Px),
	"padding":             units.NewValue(2, units.Px),
	"box-shadow.h-offset": units.NewValue(0, units.Px),
	"box-shadow.v-offset": units.NewValue(0, units.Px),
	"box-shadow.blur":     units.NewValue(0, units.Px),
	"box-shadow.color":    &Prefs.Colors.Shadow,
}

// PopupTooltip pops up a viewport displaying the tooltip text
func PopupTooltip(tooltip string, x, y int, parVp *Viewport2D, name string) *Viewport2D {
	win := parVp.Win
	mainVp := win.Viewport
	pvp := Viewport2D{}
	pvp.InitName(&pvp, name+"Tooltip")
	pvp.Win = win
	updt := pvp.UpdateStart()
	pvp.SetProp("color", &Prefs.Colors.Font)
	pvp.Fill = false
	bitflag.Set(&pvp.Flag, int(VpFlagPopup))
	bitflag.Set(&pvp.Flag, int(VpFlagTooltip))

	pvp.Geom.Pos = image.Point{x, y}
	bitflag.Set(&pvp.Flag, int(VpFlagPopupDestroyAll)) // nuke it all
	frame := pvp.AddNewChild(KiT_Frame, "Frame").(*Frame)
	frame.Lay = LayoutVert
	frame.SetProps(TooltipFrameProps, false)
	lbl := frame.AddNewChild(KiT_Label, "ttlbl").(*Label)
	lbl.SetProp("background-color", &Prefs.Colors.Highlight)
	lbl.SetProp("word-wrap", true)

	mwdots := parVp.Sty.UnContext.ToDots(40, units.Em)
	mwdots = Min32(mwdots, float32(mainVp.Geom.Size.X-20))

	lbl.SetProp("max-width", units.NewValue(mwdots, units.Dot))
	lbl.Text = tooltip
	frame.Init2DTree()
	frame.Style2DTree()                                // sufficient to get sizes
	frame.LayData.AllocSize = mainVp.LayData.AllocSize // give it the whole vp initially
	frame.Size2DTree()                                 // collect sizes
	pvp.Win = nil
	vpsz := frame.LayData.Size.Pref.Min(mainVp.LayData.AllocSize).ToPoint()
	x = kit.MinInt(x, mainVp.Geom.Size.X-vpsz.X) // fit
	y = kit.MinInt(y, mainVp.Geom.Size.Y-vpsz.Y) // fit
	pvp.Resize(vpsz)
	pvp.Geom.Pos = image.Point{x, y}
	pvp.UpdateEndNoSig(updt)

	win.PushPopup(pvp.This)
	return &pvp
}

// WidgetSignals are general signals that all widgets can send, via WidgetSig
// signal
type WidgetSignals int64

const (
	// WidgetSelected is triggered when a widget is selected, typically via
	// left mouse button click (see EmitSelectedSignal) -- is NOT contingent
	// on actual IsSelected status -- just reports the click event
	WidgetSelected WidgetSignals = iota

	// WidgetFocused is triggered when a widget receives keyboard focus (see
	// EmitFocusedSignal -- call in FocusChanged2D for gotFocus
	WidgetFocused

	// WidgetContextMenu is triggered when a widget receives a
	// right-mouse-button press, BEFORE generating and displaying the context
	// menu, so that relevant state can be updated etc (see
	// EmitContextMenuSignal)
	WidgetContextMenu

	WidgetSignalsN
)

//go:generate stringer -type=WidgetSignals

// EmitSelectedSignal emits the WidgetSelected signal for this widget
func (g *WidgetBase) EmitSelectedSignal() {
	g.WidgetSig.Emit(g.This, int64(WidgetSelected), nil)
}

// EmitFocusedSignal emits the WidgetFocused signal for this widget
func (g *WidgetBase) EmitFocusedSignal() {
	g.WidgetSig.Emit(g.This, int64(WidgetFocused), nil)
}

// EmitContextMenuSignal emits the WidgetContextMenu signal for this widget
func (g *WidgetBase) EmitContextMenuSignal() {
	g.WidgetSig.Emit(g.This, int64(WidgetContextMenu), nil)
}

// HoverTooltipEvent connects to HoverEvent and pops up a tooltip -- most
// widgets should call this as part of their event connection method
func (g *WidgetBase) HoverTooltipEvent() {
	g.ConnectEventType(oswin.MouseHoverEvent, RegPri, func(recv, send ki.Ki, sig int64, d interface{}) {
		me := d.(*mouse.HoverEvent)
		me.SetProcessed()
		ab := recv.Embed(KiT_WidgetBase).(*WidgetBase)
		if ab.Tooltip != "" {
			pos := ab.WinBBox.Max
			pos.X -= 20
			PopupTooltip(ab.Tooltip, pos.X, pos.Y, g.Viewport, ab.Nm)
		}
	})
}

// WidgetMouseEvents connects to eiher or both mouse events -- IMPORTANT: if
// you need to also connect to other mouse events, you must copy this code --
// all processing of a mouse event must happen within one function b/c there
// can only be one registered per receiver and event type.  sel = Left button
// mouse.Press event, toggles the selected state, and emits a SelectedEvent.
// ctxtMenu = connects to Right button mouse.Press event, and sends a
// WidgetSig WidgetContextMenu signal, followed by calling ContextMenu method
// -- signal can be used to change state prior to generating context menu,
// including setting a CtxtMenuFunc that removes all items and thus negates
// the presentation of any menu
func (g *WidgetBase) WidgetMouseEvents(sel, ctxtMenu bool) {
	if !sel && !ctxtMenu {
		return
	}
	g.ConnectEventType(oswin.MouseEvent, RegPri, func(recv, send ki.Ki, sig int64, d interface{}) {
		me := d.(*mouse.Event)
		if sel {
			if me.Action == mouse.Press && me.Button == mouse.Left {
				me.SetProcessed()
				ab := recv.Embed(KiT_WidgetBase).(*WidgetBase)
				ab.SetSelectedState(!ab.IsSelected())
				ab.EmitSelectedSignal()
				ab.UpdateSig()
			}
		}
		if ctxtMenu {
			if me.Action == mouse.Release && me.Button == mouse.Right {
				me.SetProcessed()
				ab := recv.Embed(KiT_WidgetBase).(*WidgetBase)
				ab.EmitContextMenuSignal()
				ab.This.(Node2D).ContextMenu()
			}
		}
	})
}

////////////////////////////////////////////////////////////////////////////////
//  Standard rendering

// RenderBoxImpl implements the standard box model rendering -- assumes all
// paint params have already been set
func (g *WidgetBase) RenderBoxImpl(pos Vec2D, sz Vec2D, rad float32) {
	rs := &g.Viewport.Render
	pc := &rs.Paint
	if rad == 0.0 {
		pc.DrawRectangle(rs, pos.X, pos.Y, sz.X, sz.Y)
	} else {
		pc.DrawRoundedRectangle(rs, pos.X, pos.Y, sz.X, sz.Y, rad)
	}
	pc.FillStrokeClear(rs)
}

// RenderStdBox draws standard box using given style
func (g *WidgetBase) RenderStdBox(st *Style) {
	rs := &g.Viewport.Render
	pc := &rs.Paint

	pos := g.LayData.AllocPos.AddVal(st.Layout.Margin.Dots)
	sz := g.LayData.AllocSize.AddVal(-2.0 * st.Layout.Margin.Dots)

	// first do any shadow
	if st.BoxShadow.HasShadow() {
		spos := pos.Add(Vec2D{st.BoxShadow.HOffset.Dots, st.BoxShadow.VOffset.Dots})
		pc.StrokeStyle.SetColor(nil)
		pc.FillStyle.Color.SetShadowGradient(st.BoxShadow.Color, "")
		// todo: this is not rendering a transparent gradient
		// pc.FillStyle.Opacity = .5
		g.RenderBoxImpl(spos, sz, st.Border.Radius.Dots)
		// pc.FillStyle.Opacity = 1.0
	}
	// then draw the box over top of that -- note: won't work well for transparent! need to set clipping to box first..
	if !st.Font.BgColor.IsNil() {
		pc.FillBox(rs, pos, sz, &st.Font.BgColor)
	}

	pc.StrokeStyle.SetColor(&st.Border.Color)
	pc.StrokeStyle.Width = st.Border.Width
	// pc.FillStyle.SetColor(&st.Font.BgColor)
	pc.FillStyle.SetColor(nil)
	g.RenderBoxImpl(pos, sz, st.Border.Radius.Dots)
}

// set our LayData.AllocSize from constraints
func (g *WidgetBase) Size2DFromWH(w, h float32) {
	st := &g.Sty
	if st.Layout.Width.Dots > 0 {
		w = Max32(st.Layout.Width.Dots, w)
	}
	if st.Layout.Height.Dots > 0 {
		h = Max32(st.Layout.Height.Dots, h)
	}
	spc := st.BoxSpace()
	w += 2.0 * spc
	h += 2.0 * spc
	g.LayData.AllocSize = Vec2D{w, h}
}

// Size2DAddSpace adds space to existing AllocSize
func (g *WidgetBase) Size2DAddSpace() {
	spc := g.Sty.BoxSpace()
	g.LayData.AllocSize.SetAddVal(2 * spc)
}

// Size2DSubSpace returns AllocSize minus 2 * BoxSpace -- the amount avail to the internal elements
func (g *WidgetBase) Size2DSubSpace() Vec2D {
	spc := g.Sty.BoxSpace()
	return g.LayData.AllocSize.SubVal(2 * spc)
}

// SetMinPrefWidth sets minimum and preferred width -- will get at least this
// amount -- max unspecified
func (g *WidgetBase) SetMinPrefWidth(val units.Value) {
	g.SetProp("width", val)
	g.SetProp("min-width", val)
}

// SetMinPrefHeight sets minimum and preferred height -- will get at least this
// amount -- max unspecified
func (g *WidgetBase) SetMinPrefHeight(val units.Value) {
	g.SetProp("height", val)
	g.SetProp("min-height", val)
}

// SetStretchMaxWidth sets stretchy max width (-1) -- can grow to take up avail room
func (g *WidgetBase) SetStretchMaxWidth() {
	g.SetProp("max-width", units.NewValue(-1, units.Px))
}

// SetStretchMaxHeight sets stretchy max height (-1) -- can grow to take up avail room
func (g *WidgetBase) SetStretchMaxHeight() {
	g.SetProp("max-height", units.NewValue(-1, units.Px))
}

// SetFixedWidth sets all width options (width, min-width, max-width) to a fixed width value
func (g *WidgetBase) SetFixedWidth(val units.Value) {
	g.SetProp("width", val)
	g.SetProp("min-width", val)
	g.SetProp("max-width", val)
}

// SetFixedHeight sets all height options (height, min-height, max-height) to
// a fixed height value
func (g *WidgetBase) SetFixedHeight(val units.Value) {
	g.SetProp("height", val)
	g.SetProp("min-height", val)
	g.SetProp("max-height", val)
}

///////////////////////////////////////////////////////////////////
// PartsWidgetBase

// PartsWidgetBase is the base type for all Widget Node2D elements that manage
// a set of constitutent parts
type PartsWidgetBase struct {
	WidgetBase
	Parts Layout `json:"-" xml:"-" view-closed:"true" desc:"a separate tree of sub-widgets that implement discrete parts of a widget -- positions are always relative to the parent widget -- fully managed by the widget and not saved"`
}

var KiT_PartsWidgetBase = kit.Types.AddType(&PartsWidgetBase{}, PartsWidgetBaseProps)

var PartsWidgetBaseProps = ki.Props{
	"base-type": true,
}

// standard FunDownMeFirst etc operate automaticaly on Field structs such as
// Parts -- custom calls only needed for manually-recursive traversal in
// Layout and Render

// SizeFromParts sets our size from those of our parts -- default..
func (g *PartsWidgetBase) SizeFromParts() {
	g.LayData.AllocSize = g.Parts.LayData.Size.Pref // get from parts
	g.Size2DAddSpace()
	if Layout2DTrace {
		fmt.Printf("Size:   %v size from parts: %v, parts pref: %v\n", g.PathUnique(), g.LayData.AllocSize, g.Parts.LayData.Size.Pref)
	}
}

func (g *PartsWidgetBase) Size2DParts() {
	g.InitLayout2D()
	g.SizeFromParts() // get our size from parts
}

func (g *PartsWidgetBase) Size2D() {
	g.Size2DParts()
}

func (g *PartsWidgetBase) ComputeBBox2DParts(parBBox image.Rectangle, delta image.Point) {
	g.ComputeBBox2DBase(parBBox, delta)
	g.Parts.This.(Node2D).ComputeBBox2D(parBBox, delta)
}

func (g *PartsWidgetBase) ComputeBBox2D(parBBox image.Rectangle, delta image.Point) {
	g.ComputeBBox2DParts(parBBox, delta)
}

func (g *PartsWidgetBase) Layout2DParts(parBBox image.Rectangle) {
	spc := g.Sty.BoxSpace()
	g.Parts.LayData.AllocPos = g.LayData.AllocPos.AddVal(spc)
	g.Parts.LayData.AllocSize = g.LayData.AllocSize.AddVal(-2.0 * spc)
	g.Parts.Layout2D(parBBox)
}

func (g *PartsWidgetBase) Layout2D(parBBox image.Rectangle) {
	g.Layout2DBase(parBBox, true) // init style
	g.Layout2DParts(parBBox)
	g.Layout2DChildren()
}

func (g *PartsWidgetBase) Render2DParts() {
	g.Parts.Render2DTree()
}

func (g *PartsWidgetBase) Move2D(delta image.Point, parBBox image.Rectangle) {
	g.Move2DBase(delta, parBBox)
	g.Parts.This.(Node2D).Move2D(delta, parBBox)
	g.Move2DChildren(delta)
}

///////////////////////////////////////////////////////////////////
// ConfigParts building-blocks

// ConfigPartsIconLabel returns a standard config for creating parts, of icon
// and label left-to right in a row, based on whether items are nil or empty
func (g *PartsWidgetBase) ConfigPartsIconLabel(icnm string, txt string) (config kit.TypeAndNameList, icIdx, lbIdx int) {
	// todo: add some styles for button layout
	config = kit.TypeAndNameList{}
	icIdx = -1
	lbIdx = -1
	if IconName(icnm).IsValid() {
		config.Add(KiT_Icon, "icon")
		icIdx = 0
		if txt != "" {
			config.Add(KiT_Space, "space")
		}
	}
	if txt != "" {
		lbIdx = len(config)
		config.Add(KiT_Label, "label")
	}
	return
}

// ConfigPartsSetIconLabel sets the icon and text values in parts, and get
// part style props, using given props if not set in object props
func (g *PartsWidgetBase) ConfigPartsSetIconLabel(icnm string, txt string, icIdx, lbIdx int) {
	if icIdx >= 0 {
		ic := g.Parts.KnownChild(icIdx).(*Icon)
		if set, _ := ic.SetIcon(icnm); set || g.NeedsFullReRender() {
			g.StylePart(Node2D(ic))
		}
	}
	if lbIdx >= 0 {
		lbl := g.Parts.KnownChild(lbIdx).(*Label)
		if lbl.Text != txt {
			g.StylePart(Node2D(lbl))
			if icIdx >= 0 {
				g.StylePart(g.Parts.KnownChild(lbIdx - 1).(Node2D)) // also get the space
			}
			lbl.SetText(txt)
		}
	}
}

// PartsNeedUpdateIconLabel check if parts need to be updated -- for ConfigPartsIfNeeded
func (g *PartsWidgetBase) PartsNeedUpdateIconLabel(icnm string, txt string) bool {
	if IconName(icnm).IsValid() {
		ick, ok := g.Parts.ChildByName("icon", 0)
		if !ok {
			return true
		}
		ic := ick.(*Icon)
		if !ic.HasChildren() || ic.UniqueNm != icnm || g.NeedsFullReRender() {
			return true
		}
	} else {
		_, ok := g.Parts.ChildByName("icon", 0)
		if ok { // need to remove it
			return true
		}
	}
	if txt != "" {
		lbl, ok := g.Parts.ChildByName("label", 2)
		if !ok {
			return true
		}
		lbl.(*Label).Sty.Font.Color = g.Sty.Font.Color
		if lbl.(*Label).Text != txt {
			return true
		}
	} else {
		_, ok := g.Parts.ChildByName("label", 2)
		if ok {
			return true
		}
	}
	return false
}
