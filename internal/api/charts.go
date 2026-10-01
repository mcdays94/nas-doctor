package api

// ChartJS is a self-contained vanilla JavaScript charting library that renders
// to HTML5 Canvas elements. It supports line, area, bar, gauge, and sparkline
// chart types with hover tooltips, smooth curves, animation, and dark-mode
// auto-detection. Zero external dependencies.
var ChartJS = `
(function(){
"use strict";

/* ── helpers ─────────────────────────────────────────────────────── */
function isDarkMode(){
  if(window.matchMedia && window.matchMedia("(prefers-color-scheme:dark)").matches) return true;
  var bg=getComputedStyle(document.body).backgroundColor;
  var m=bg.match(/^rgb\((\d+)/);
  if(m&&parseInt(m[1],10)<50) return true;
  return false;
}
function theme(){
  var dk=isDarkMode();
  return {
    grid:    dk?"rgba(255,255,255,0.06)":"rgba(0,0,0,0.06)",
    text:    dk?"rgba(255,255,255,0.50)":"rgba(0,0,0,0.45)",
    tooltip: dk?"rgba(30,30,30,0.92)":"rgba(255,255,255,0.95)",
    tipText: dk?"#e0e0e0":"#222",
    tipBord: dk?"rgba(255,255,255,0.12)":"rgba(0,0,0,0.10)"
  };
}

function prepCanvas(id,opts){
  var el=typeof id==="string"?document.getElementById(id):id;
  if(!el) return null;
  var parent=el.parentElement||document.body;
  var pw=parent.clientWidth||300;
  var cs=getComputedStyle(parent);
  pw-=(parseFloat(cs.paddingLeft)||0)+(parseFloat(cs.paddingRight)||0);
  var fluid=!!(opts&&opts.fluid);
  var w=opts&&opts.width&&!fluid?opts.width:pw;
  var h=opts&&opts.height?opts.height:el.getAttribute("height")?parseInt(el.getAttribute("height"),10):200;
  var dpr=window.devicePixelRatio||1;
  el.width=w*dpr; el.height=h*dpr;
  el.style.width=fluid?"100%":w+"px"; el.style.height=h+"px";
  var ctx=el.getContext("2d");
  ctx.scale(dpr,dpr);
  el._nasDraw=(el._nasDraw||0)+1;
  return {el:el,ctx:ctx,w:w,h:h,dpr:dpr,draw:el._nasDraw};
}

/* ── fluid charts ────────────────────────────────────────────────
   opts.fluid sizes a chart to its parent's content box and redraws it,
   without the intro animation, whenever that box changes width: a
   window resize, or a dashboard card that drops to one column on a
   narrow screen or moves to another column. The canvas stays at 100%
   width so it never holds its card open at the old size. */
var fluidWatch=null, fluidEls=[];
function keepFluid(c,draw,opts){
  var el=c.el;
  if(!opts||!opts.fluid||typeof ResizeObserver==="undefined"){
    /* A fixed-size redraw must not be resized back to old data. */
    if(el._nasFluid) el._nasFluid=null;
    return;
  }
  el._nasFluid={w:Math.round(c.w),redraw:function(){
    var o={}; for(var k in opts) o[k]=opts[k];
    o.animate=false;
    draw(el,o);
  }};
  if(!fluidWatch) fluidWatch=new ResizeObserver(function(entries){
    for(var i=0;i<entries.length;i++){
      var f=entries[i].target._nasFluid;
      var w=Math.round(entries[i].contentRect.width);
      if(f&&w>0&&w!==f.w) f.redraw();
    }
  });
  if(fluidEls.indexOf(el)>=0) return;
  /* Let go of canvases a dashboard re-render has replaced. */
  fluidEls=fluidEls.filter(function(x){
    if(x.isConnected) return true;
    fluidWatch.unobserve(x);
    return false;
  });
  fluidEls.push(el);
  fluidWatch.observe(el);
}

/* Plays the intro animation, or draws once when opts.animate is false.
   A newer draw on the same canvas (a resize, a range button) stops an
   older animation so it can't paint over the new chart. */
function play(c,render,opts){
  if(opts&&opts.animate===false){render(1);return;}
  animate(500,function(t){if(c.el._nasDraw===c.draw) render(t);});
}

function clamp(v,lo,hi){return v<lo?lo:v>hi?hi:v;}

function lerp(a,b,t){return a+(b-a)*t;}

function drawSmooth(ctx,pts,close){
  if(pts.length<2) return;
  ctx.moveTo(pts[0].x,pts[0].y);
  if(pts.length===2){ctx.lineTo(pts[1].x,pts[1].y);return;}
  for(var i=0;i<pts.length-1;i++){
    var cx=(pts[i].x+pts[i+1].x)/2;
    var cy=(pts[i].y+pts[i+1].y)/2;
    if(i===0) ctx.lineTo(cx,cy);
    else ctx.quadraticCurveTo(pts[i].x,pts[i].y,cx,cy);
  }
  var last=pts[pts.length-1];
  ctx.quadraticCurveTo(last.x,last.y,last.x,last.y);
}

/* ── animation helper ────────────────────────────────────────────── */
function animate(dur,fn,done){
  var start=null;
  function step(ts){
    if(!start)start=ts;
    var t=clamp((ts-start)/dur,0,1);
    fn(t);
    if(t<1) requestAnimationFrame(step);
    else if(done) done();
  }
  requestAnimationFrame(step);
}

/* ── tooltip helper ──────────────────────────────────────────────── */
function attachTooltip(el,hitTest){
  /* A redraw on the same canvas swaps in its hit test. Another tooltip
     and listener pair would leave the old chart answering hovers. */
  if(el._nasTip){el._nasTip.hitTest=hitTest;return el._nasTip.cross;}
  var tip=document.createElement("div");
  tip.style.cssText="position:fixed;padding:6px 10px;border-radius:6px;font:11px/1.4 -apple-system,system-ui,sans-serif;pointer-events:none;opacity:0;transition:opacity .15s;z-index:9999;max-width:220px;white-space:nowrap;";
  document.body.appendChild(tip);
  var cross={x:-1,active:false};
  var state=el._nasTip={hitTest:hitTest,cross:cross};

  el.addEventListener("mousemove",function(e){
    var r=el.getBoundingClientRect();
    var x=e.clientX-r.left, y=e.clientY-r.top;
    var info=state.hitTest(x,y,cross);
    if(!info){tip.style.opacity="0";cross.active=false;return;}
    var th=theme();
    tip.style.background=th.tooltip;
    tip.style.color=th.tipText;
    tip.style.border="1px solid "+th.tipBord;
    tip.style.boxShadow="0 4px 12px rgba(0,0,0,0.15)";
    tip.innerHTML=info.html;
    cross.x=info.cx!==undefined?info.cx:x;
    cross.active=true;
    var tx=e.clientX+12, ty=e.clientY-10;
    if(tx+220>window.innerWidth) tx=e.clientX-230;
    tip.style.left=tx+"px";
    tip.style.top=ty+"px";
    tip.style.opacity="1";
    if(info.redraw) info.redraw();
  });

  el.addEventListener("mouseleave",function(){
    tip.style.opacity="0";
    cross.active=false;
    var info=state.hitTest(-1,-1,cross);
    if(info&&info.redraw) info.redraw();
  });
  return cross;
}

/* ── MARGINS ─────────────────────────────────────────────────────── */
function margins(opts,h){
  /* Adapt margins for small canvases (e.g. 60-80px sparkline-style charts) */
  var compact=h!==undefined&&h<=100;
  var l=compact?32:(opts&&opts.yLabel?52:44);
  var b=compact?18:(opts&&opts.xLabel?42:34);
  var t=compact?8:16;
  return {t:t,r:compact?8:16,b:b,l:l};
}

/* ── computeYTicks ───────────────────────────────────────────────── */
function computeYTicks(dMin,dMax,yMax,count){
  var mn=dMin,mx=yMax!==undefined?yMax:dMax;
  if(mn===mx){mn=mn-1;mx=mx+1;}
  var range=mx-mn;
  var step=range/(count-1);
  var ticks=[];
  for(var i=0;i<count;i++) ticks.push(mn+step*i);
  return {min:mn,max:mx,ticks:ticks};
}

/* ── decimateLabels ──────────────────────────────────────────────
   Given N label slots spread evenly across chart-pixel-width cw, the
   widest rendered label width maxLabelWidth, and a minimum horizontal
   gap minGap between adjacent labels, return the set of label indices
   to render without any two labels overlapping.

   Guarantees:
     - Index 0 (first) is always present when N>=1.
     - Index N-1 (last) is always present when N>=2.
     - No two consecutive returned indices i<j produce rendered labels
       whose pixel bounding boxes intersect. (Spacing between their
       centers is stride*(cw/(N-1)) which is >= maxLabelWidth+minGap
       by construction — except for the final "snap to last" entry,
       which is guaranteed spacing >= maxLabelWidth+minGap because
       we drop the previous anchor if it would collide with the last.)

   Edge cases:
     - N<=1         → [0] (or [] if N=0)
     - cw<=0 or L=0 → just [0, N-1] (degenerate, no width info)
     - L+G >= cw    → [0, N-1] (only first and last fit)

   This is exported as a pure function on NasChart so it can be unit-
   tested in isolation (see internal/api/charts_decimation_test.go and
   scripts/charts_decimation.test.js). Issue #165. */
function decimateLabels(n,cw,maxLabelWidth,minGap){
  if(minGap==null) minGap=12;
  if(!(n>0)) return [];
  if(n===1) return [0];
  if(!(cw>0)||!(maxLabelWidth>0)) return [0,n-1];
  var per=maxLabelWidth+minGap;
  var intervals=n-1;
  /* How many labels fit? (cw + gap) / (label + gap) because the last
     label doesn't need a trailing gap. */
  var maxLabels=Math.floor((cw+minGap)/per);
  if(maxLabels<2) return [0,n-1];
  if(maxLabels>=n) {
    /* every label fits */
    var out=[];
    for(var i=0;i<n;i++) out.push(i);
    return out;
  }
  /* Smallest stride s such that s*(cw/intervals) >= per. */
  var stride=Math.max(1,Math.ceil(per*intervals/cw));
  var idx=[];
  for(var j=0;j<n;j+=stride) idx.push(j);
  /* Always include the last label. Drop the previous anchor if it
     would collide with the last (pixel distance < per). */
  var last=n-1;
  if(idx[idx.length-1]!==last){
    var lastAnchor=idx[idx.length-1];
    var gapPx=(last-lastAnchor)*(cw/intervals);
    if(gapPx<per) idx.pop();
    idx.push(last);
  }
  return idx;
}

/* ── drawAxes ────────────────────────────────────────────────────── */
function drawAxes(ctx,m,w,h,yInfo,labels,opts){
  var th=theme();
  var cw=w-m.l-m.r, ch=h-m.t-m.b;
  /* y grid + labels */
  ctx.font="11px -apple-system,system-ui,sans-serif";
  ctx.textAlign="right"; ctx.textBaseline="middle";
  for(var i=0;i<yInfo.ticks.length;i++){
    var vy=yInfo.ticks[i];
    var py=m.t+ch-(vy-yInfo.min)/(yInfo.max-yInfo.min)*ch;
    ctx.strokeStyle=th.grid; ctx.lineWidth=1;
    ctx.setLineDash([4,4]); ctx.beginPath();
    ctx.moveTo(m.l,py); ctx.lineTo(w-m.r,py); ctx.stroke();
    ctx.setLineDash([]);
    ctx.fillStyle=th.text;
    var lbl=vy%1===0?vy.toString():vy.toFixed(1);
    ctx.fillText(lbl,m.l-8,py);
  }
  /* x labels — decimate to prevent overlap. Measure real label widths
     via ctx.measureText() and derive which indices to render from
     decimateLabels(). Fixes issue #165: the old
       step = floor(labels.length / (cw / 50))
     hardcoded a ~50px label budget, so datetime labels like "4/17 23:11"
     (~65-75px in 11px sans-serif) would collide into an unreadable wall
     of overlapping text once enough history accumulated. */
  if(labels&&labels.length){
    ctx.textAlign="center"; ctx.textBaseline="top";
    var maxLabelWidth=0;
    for(var mi=0;mi<labels.length;mi++){
      var lw=ctx.measureText(labels[mi]==null?"":String(labels[mi])).width;
      if(lw>maxLabelWidth) maxLabelWidth=lw;
    }
    var keep=decimateLabels(labels.length,cw,maxLabelWidth,12);
    var intervals=labels.length-1||1;
    for(var ki=0;ki<keep.length;ki++){
      var j=keep[ki];
      var px=m.l+j/intervals*cw;
      ctx.fillStyle=th.text;
      ctx.fillText(labels[j],px,h-m.b+8);
    }
  }
  /* axis titles */
  if(opts&&opts.yLabel){
    ctx.save(); ctx.translate(12,m.t+ch/2);
    ctx.rotate(-Math.PI/2); ctx.textAlign="center";
    ctx.fillStyle=th.text; ctx.fillText(opts.yLabel,0,0);
    ctx.restore();
  }
  if(opts&&opts.xLabel){
    ctx.textAlign="center"; ctx.fillStyle=th.text;
    ctx.fillText(opts.xLabel,m.l+cw/2,h-4);
  }
}

/* ── drawCompactAxes (for small charts ≤100px) ───────────────────── */
function drawCompactAxes(ctx,m,w,h,yInfo,opts){
  var th=theme();
  var ch=h-m.t-m.b;
  ctx.font="10px -apple-system,system-ui,sans-serif";
  ctx.textAlign="right"; ctx.textBaseline="middle";
  /* Only draw min and max labels */
  var ticks=[yInfo.ticks[0],yInfo.ticks[yInfo.ticks.length-1]];
  for(var i=0;i<ticks.length;i++){
    var vy=ticks[i];
    var py=m.t+ch-(vy-yInfo.min)/(yInfo.max-yInfo.min)*ch;
    ctx.fillStyle=th.text;
    var lbl=vy%1===0?vy.toString():vy.toFixed(1);
    ctx.fillText(lbl,m.l-6,py);
  }
  /* light baseline */
  ctx.strokeStyle=th.grid; ctx.lineWidth=1;
  ctx.setLineDash([3,3]); ctx.beginPath();
  ctx.moveTo(m.l,m.t+ch); ctx.lineTo(w-m.r,m.t+ch); ctx.stroke();
  ctx.setLineDash([]);
  /* Y-axis label */
  if(opts&&opts.yLabel){
    ctx.save(); ctx.translate(8,m.t+ch/2);
    ctx.rotate(-Math.PI/2); ctx.textAlign="center";
    ctx.fillStyle=th.text; ctx.font="9px -apple-system,system-ui,sans-serif";
    ctx.fillText(opts.yLabel,0,0);
    ctx.restore();
  }
}

/* ── crosshair helper ────────────────────────────────────────────── */
function drawCrosshair(ctx,cross,m,h){
  if(!cross.active||cross.x<0) return;
  var th=theme();
  ctx.strokeStyle=th.text; ctx.lineWidth=0.5;
  ctx.setLineDash([3,3]); ctx.beginPath();
  ctx.moveTo(cross.x,m.t); ctx.lineTo(cross.x,h-m.b);
  ctx.stroke(); ctx.setLineDash([]);
}

/* ── computePoints ───────────────────────────────────────────────── */
function computePoints(data,m,cw,ch,yInfo){
  var pts=[];
  for(var i=0;i<data.length;i++){
    var x=m.l+(data.length===1?cw/2:i/(data.length-1)*cw);
    var y=m.t+ch-(data[i]-yInfo.min)/(yInfo.max-yInfo.min)*ch;
    pts.push({x:x,y:y,v:data[i]});
  }
  return pts;
}

/* ─── NasChart.line ──────────────────────────────────────────────── */
function drawLine(id,opts){
  var c=prepCanvas(id,opts); if(!c) return;
  var ctx=c.ctx, w=c.w, h=c.h;
  var m=margins(opts,h), cw=w-m.l-m.r, ch=h-m.t-m.b;
  var ds=opts.datasets||[];
  var allD=[]; ds.forEach(function(d){allD=allD.concat(d.data);});
  var yInfo=computeYTicks(Math.min.apply(null,allD),Math.max.apply(null,allD),opts.yMax,6);
  var allPts=ds.map(function(d){return computePoints(d.data,m,cw,ch,yInfo);});
  var cross={x:-1,active:false};

  function render(progress){
    ctx.clearRect(0,0,w,h);
    drawAxes(ctx,m,w,h,yInfo,opts.labels,opts);
    drawCrosshair(ctx,cross,m,h);
    var pIdx=progress!==undefined?progress:1;
    ds.forEach(function(d,di){
      var pts=allPts[di];
      var count=Math.max(2,Math.ceil(pts.length*pIdx));
      var visible=pts.slice(0,count);
      ctx.strokeStyle=d.color||"#55b3ff";
      ctx.lineWidth=2; ctx.lineJoin="round";
      if(d.dashed) ctx.setLineDash([6,4]);
      else ctx.setLineDash([]);
      ctx.beginPath(); drawSmooth(ctx,visible); ctx.stroke();
      ctx.setLineDash([]);
      /* dots */
      visible.forEach(function(p){
        ctx.beginPath(); ctx.arc(p.x,p.y,3,0,Math.PI*2);
        ctx.fillStyle=d.color||"#55b3ff"; ctx.fill();
      });
    });
  }

  play(c,render,opts);
  keepFluid(c,drawLine,opts);

  attachTooltip(c.el,function(mx,my,cr){
    cross.active=cr.active; cross.x=cr.x;
    if(mx<m.l||mx>w-m.r) return null;
    var labels=opts.labels||[];
    var idx=Math.round((mx-m.l)/cw*(labels.length-1));
    idx=clamp(idx,0,labels.length-1);
    var cx=m.l+idx/(labels.length-1||1)*cw;
    var rows=ds.map(function(d,di){
      var v=d.data[idx];
      return "<div style='margin:2px 0'><span style='display:inline-block;width:8px;height:8px;border-radius:50%;background:"+(d.color||"#55b3ff")+";margin-right:6px'></span>"+
        (d.label||"Series "+(di+1))+": <b>"+v+"</b></div>";
    }).join("");
    var html="<div style='font-weight:600;margin-bottom:4px'>"+(labels[idx]||"")+"</div>"+rows;
    return {html:html,cx:cx,redraw:function(){render(1);}};
  });
}

/* ─── NasChart.area ──────────────────────────────────────────────── */
function drawArea(id,opts){
  var c=prepCanvas(id,opts); if(!c) return;
  var ctx=c.ctx, w=c.w, h=c.h;
  var m=margins(opts,h), cw=w-m.l-m.r, ch=h-m.t-m.b;
  var compact=h<=100;
  var ds=opts.datasets||[];
  var allD=[]; ds.forEach(function(d){allD=allD.concat(d.data);});
  var tickCount=compact?3:6;
  var yInfo=computeYTicks(Math.min.apply(null,allD),Math.max.apply(null,allD),opts.yMax,tickCount);
  var allPts=ds.map(function(d){return computePoints(d.data,m,cw,ch,yInfo);});
  var cross={x:-1,active:false};

  function render(progress){
    ctx.clearRect(0,0,w,h);
    if(compact) drawCompactAxes(ctx,m,w,h,yInfo,opts);
    else drawAxes(ctx,m,w,h,yInfo,opts.labels,opts);
    drawCrosshair(ctx,cross,m,h);
    var pIdx=progress!==undefined?progress:1;
    ds.forEach(function(d,di){
      var pts=allPts[di];
      var count=Math.max(2,Math.ceil(pts.length*pIdx));
      var visible=pts.slice(0,count);
      var col=d.color||"#55b3ff";
      /* fill */
      var grad=ctx.createLinearGradient(0,m.t,0,m.t+ch);
      grad.addColorStop(0,col.replace(")",",0.3)").replace("rgb","rgba").replace("##","#"));
      /* hex to rgba for gradient */
      var r=parseInt(col.slice(1,3),16),g=parseInt(col.slice(3,5),16),b=parseInt(col.slice(5,7),16);
      grad.addColorStop(0,"rgba("+r+","+g+","+b+",0.3)");
      grad.addColorStop(1,"rgba("+r+","+g+","+b+",0.0)");
      ctx.beginPath(); drawSmooth(ctx,visible);
      ctx.lineTo(visible[visible.length-1].x,m.t+ch);
      ctx.lineTo(visible[0].x,m.t+ch); ctx.closePath();
      ctx.fillStyle=grad; ctx.fill();
      /* stroke */
      ctx.strokeStyle=col; ctx.lineWidth=2; ctx.lineJoin="round";
      ctx.beginPath(); drawSmooth(ctx,visible); ctx.stroke();
      /* dots */
      visible.forEach(function(p){
        ctx.beginPath(); ctx.arc(p.x,p.y,3,0,Math.PI*2);
        ctx.fillStyle=col; ctx.fill();
      });
    });
  }

  play(c,render,opts);
  keepFluid(c,drawArea,opts);

  attachTooltip(c.el,function(mx,my,cr){
    cross.active=cr.active; cross.x=cr.x;
    if(mx<m.l||mx>w-m.r) return null;
    var labels=opts.labels||[];
    var idx=Math.round((mx-m.l)/cw*(labels.length-1));
    idx=clamp(idx,0,labels.length-1);
    var cx=m.l+idx/(labels.length-1||1)*cw;
    var rows=ds.map(function(d,di){
      var v=d.data[idx]; var col=d.color||"#55b3ff";
      return "<div style='margin:2px 0'><span style='display:inline-block;width:8px;height:8px;border-radius:50%;background:"+col+";margin-right:6px'></span>"+
        (d.label||"Series "+(di+1))+": <b>"+v+"</b></div>";
    }).join("");
    var html="<div style='font-weight:600;margin-bottom:4px'>"+(labels[idx]||"")+"</div>"+rows;
    return {html:html,cx:cx,redraw:function(){render(1);}};
  });
}

/* ─── NasChart.bar ───────────────────────────────────────────────── */
function drawBar(id,opts){
  var c=prepCanvas(id,opts); if(!c) return;
  var ctx=c.ctx, w=c.w, h=c.h;
  var m=margins(opts,h), cw=w-m.l-m.r, ch=h-m.t-m.b;
  var data=opts.data||[];
  var labels=opts.labels||[];
  var colors=opts.colors||[];
  var defCol="#55b3ff";
  var yInfo=computeYTicks(0,Math.max.apply(null,data),opts.yMax,6);
  var cross={x:-1,active:false,idx:-1};

  function render(progress){
    ctx.clearRect(0,0,w,h);
    drawAxes(ctx,m,w,h,yInfo,labels,opts);
    var p=progress!==undefined?progress:1;
    var n=data.length; if(!n) return;
    var gap=Math.max(6,cw*0.15/(n));
    var bw=(cw-gap*(n+1))/n;
    bw=Math.min(bw,60);
    var totalW=n*bw+(n+1)*gap;
    var startX=m.l+(cw-totalW)/2+gap;
    for(var i=0;i<n;i++){
      var bh=((data[i]-yInfo.min)/(yInfo.max-yInfo.min))*ch*p;
      var x=startX+i*(bw+gap);
      var y=m.t+ch-bh;
      var col=colors[i]||defCol;
      /* bar with rounded top */
      var rad=Math.min(4,bw/4);
      ctx.beginPath();
      ctx.moveTo(x,m.t+ch);
      ctx.lineTo(x,y+rad);
      ctx.quadraticCurveTo(x,y,x+rad,y);
      ctx.lineTo(x+bw-rad,y);
      ctx.quadraticCurveTo(x+bw,y,x+bw,y+rad);
      ctx.lineTo(x+bw,m.t+ch);
      ctx.closePath();
      ctx.fillStyle=col; ctx.fill();
      /* hover highlight */
      if(cross.active&&cross.idx===i){
        ctx.fillStyle="rgba(255,255,255,0.12)"; ctx.fill();
      }
    }
  }

  play(c,render,opts);
  keepFluid(c,drawBar,opts);

  attachTooltip(c.el,function(mx,my,cr){
    cross.active=cr.active; cross.x=cr.x;
    var n=data.length; if(!n) return null;
    var gap=Math.max(6,cw*0.15/(n));
    var bw=(cw-gap*(n+1))/n;
    bw=Math.min(bw,60);
    var totalW=n*bw+(n+1)*gap;
    var startX=m.l+(cw-totalW)/2+gap;
    var idx=-1;
    for(var i=0;i<n;i++){
      var x=startX+i*(bw+gap);
      if(mx>=x&&mx<=x+bw){idx=i;break;}
    }
    cross.idx=idx;
    if(idx<0) return null;
    var col=colors[idx]||defCol;
    var html="<div style='font-weight:600;margin-bottom:2px'>"+(labels[idx]||"Bar "+(idx+1))+"</div>"+
      "<div><span style='display:inline-block;width:8px;height:8px;border-radius:50%;background:"+col+";margin-right:6px'></span><b>"+data[idx]+"</b></div>";
    return {html:html,cx:startX+idx*(bw+gap)+bw/2,redraw:function(){render(1);}};
  });
}

/* ─── NasChart.gauge ─────────────────────────────────────────────── */
function drawGauge(id,opts){
  var c=prepCanvas(id,opts); if(!c) return;
  var ctx=c.ctx, w=c.w, h=c.h;
  var val=opts.value||0, mx=opts.max||100;
  var label=opts.label||"";
  var pct=clamp(val/mx,0,1);

  /* auto color from thresholds */
  var col=opts.color||"#5fc992";
  if(opts.thresholds){
    var g=opts.thresholds.good!==undefined?opts.thresholds.good:80;
    var wa=opts.thresholds.warn!==undefined?opts.thresholds.warn:50;
    if(val>=g) col="#5fc992";
    else if(val>=wa) col="#ffbc33";
    else col="#FF6363";
  }

  var cx=w/2, cy=h*0.62;
  var rad=Math.min(w/2-20,h*0.55-10);
  var lw=rad*0.18;
  var th=theme();

  function render(progress){
    var p=progress!==undefined?progress:1;
    ctx.clearRect(0,0,w,h);
    /* track */
    ctx.beginPath();
    ctx.arc(cx,cy,rad,Math.PI,2*Math.PI);
    ctx.strokeStyle=th.grid; ctx.lineWidth=lw;
    ctx.lineCap="round"; ctx.stroke();
    /* value arc */
    var endAngle=Math.PI+Math.PI*pct*p;
    ctx.beginPath();
    ctx.arc(cx,cy,rad,Math.PI,endAngle);
    ctx.strokeStyle=col; ctx.lineWidth=lw;
    ctx.lineCap="round"; ctx.stroke();
    /* value text */
    ctx.fillStyle=th.text.replace("0.50","0.9").replace("0.45","0.85");
    ctx.font="bold "+Math.round(rad*0.48)+"px -apple-system,system-ui,sans-serif";
    ctx.textAlign="center"; ctx.textBaseline="middle";
    ctx.fillText(Math.round(val*p),cx,cy-rad*0.08);
    /* label */
    ctx.fillStyle=th.text;
    ctx.font="12px -apple-system,system-ui,sans-serif";
    ctx.fillText(label,cx,cy+rad*0.38);
  }

  // PRD #283 / issue #285: opts.animate=false skips the sweep so
  // live-progress redraws don't re-sweep from zero on every sample
  // tick. Default behaviour unchanged (full sweep on first render).
  if (opts.animate === false) {
    render(1);
  } else {
    animate(500,function(t){render(t);});
  }
}

/* ─── NasChart.sparkline ─────────────────────────────────────────── */
function drawSparkline(id,opts){
  var sw=opts.width||120, sh=opts.height||30;
  opts.width=sw; opts.height=sh;
  var c=prepCanvas(id,opts); if(!c) return;
  var ctx=c.ctx;
  var data=opts.data||[];
  if(!data.length) return;
  var col=opts.color||"#55b3ff";
  var mn=Math.min.apply(null,data), mx=Math.max.apply(null,data);
  if(mn===mx){mn-=1;mx+=1;}
  var pad=2;

  function render(progress){
    var p=progress!==undefined?progress:1;
    ctx.clearRect(0,0,sw,sh);
    var pts=[];
    var count=Math.max(2,Math.ceil(data.length*p));
    for(var i=0;i<count&&i<data.length;i++){
      var x=pad+i/(data.length-1)*(sw-2*pad);
      var y=sh-pad-(data[i]-mn)/(mx-mn)*(sh-2*pad);
      pts.push({x:x,y:y});
    }
    /* gradient fill */
    var r=parseInt(col.slice(1,3),16),g=parseInt(col.slice(3,5),16),b=parseInt(col.slice(5,7),16);
    var grad=ctx.createLinearGradient(0,0,0,sh);
    grad.addColorStop(0,"rgba("+r+","+g+","+b+",0.18)");
    grad.addColorStop(1,"rgba("+r+","+g+","+b+",0.0)");
    ctx.beginPath(); drawSmooth(ctx,pts);
    ctx.lineTo(pts[pts.length-1].x,sh); ctx.lineTo(pts[0].x,sh);
    ctx.closePath(); ctx.fillStyle=grad; ctx.fill();
    /* line */
    ctx.beginPath(); drawSmooth(ctx,pts);
    ctx.strokeStyle=col; ctx.lineWidth=1.5; ctx.lineJoin="round";
    ctx.stroke();
  }

  animate(500,function(t){render(t);});
}

/* ── SPEED TEST ──────────────────────────────────────────────────────
   drawSpeedTest plots one speed test: download and upload Mbps against
   seconds since the first throughput sample, as smoothed lines over a
   fading fill, with a glowing head on the series being measured and
   dashed lines at the contracted speeds when they are set. The canvas
   fills its parent.
   data: {down, up: [{t, v}], phase, contractedDown, contractedUp}
   o:    {live, compact, font, colors: {down, up, grid, text}} */
function drawSpeedTest(canvas,data,o){
  if(!canvas||!canvas.getContext) return;
  o=o||{};
  var wrap=canvas.parentNode;
  var w=wrap.clientWidth,h=wrap.clientHeight;
  if(!w||!h) return;
  var dpr=window.devicePixelRatio||1;
  if(canvas.width!==Math.round(w*dpr)||canvas.height!==Math.round(h*dpr)){
    canvas.width=Math.round(w*dpr);
    canvas.height=Math.round(h*dpr);
  }
  var ctx=canvas.getContext("2d");
  ctx.setTransform(dpr,0,0,dpr,0,0);
  ctx.clearRect(0,0,w,h);

  var col=o.colors||{};
  var colDown=col.down||"#60a5fa",colUp=col.up||"#a78bfa";
  var colGrid=col.grid||"rgba(127,127,127,0.2)",colText=col.text||"#62666d";
  var font=o.font||"system-ui, sans-serif";
  var compact=!!o.compact,live=!!o.live;
  var down=data.down||[],up=data.up||[];
  var cDown=data.contractedDown||0,cUp=data.contractedUp||0;

  var padL=compact?36:44,padR=compact?8:12,padT=compact?8:12,padB=compact?18:22;
  var cw=w-padL-padR,ch=h-padT-padB;
  var all=down.concat(up);
  var lastT=0,peak=0;
  for(var i=0;i<all.length;i++){
    if(all[i].t>lastT) lastT=all[i].t;
    if(all[i].v>peak) peak=all[i].v;
  }
  var xMax=Math.max(15,lastT+(live?1.5:0.2));
  var yMax=niceSpeedMax(Math.max(peak,cDown,cUp,10)*1.12);
  function X(t){return padL+(t/xMax)*cw;}
  function Y(v){return padT+ch-(Math.max(0,v)/yMax)*ch;}

  ctx.font=(compact?10:11)+"px "+font;
  ctx.lineWidth=1;
  ctx.strokeStyle=colGrid;
  ctx.fillStyle=colText;
  ctx.textAlign="right";
  ctx.textBaseline="middle";
  for(var g=0;g<=4;g++){
    var gv=yMax*g/4,gy=Math.round(Y(gv))+0.5;
    ctx.beginPath();ctx.moveTo(padL,gy);ctx.lineTo(w-padR,gy);ctx.stroke();
    ctx.fillText(String(Math.round(gv)),padL-(compact?6:8),gy);
  }
  ctx.textAlign="center";
  ctx.textBaseline="top";
  var step=xMax>45?10:5;
  for(var s=0;s<=xMax;s+=step) ctx.fillText(s+"s",X(s),padT+ch+(compact?4:6));

  drawRef(cDown,colDown);
  drawRef(cUp,colUp);
  if(cDown>0||cUp>0){
    /* Legend for the dashed lines, in the headroom above the data. */
    ctx.save();
    ctx.font=(compact?9:10)+"px "+font;
    ctx.textAlign="right";
    ctx.textBaseline="middle";
    ctx.fillStyle=colText;
    var lx=w-padR,ly=padT+7;
    ctx.fillText("contracted",lx,ly);
    var tw=ctx.measureText("contracted").width;
    ctx.setLineDash([3,3]);
    ctx.strokeStyle=colText;
    ctx.beginPath();ctx.moveTo(lx-tw-22,ly+0.5);ctx.lineTo(lx-tw-6,ly+0.5);ctx.stroke();
    ctx.restore();
  }
  drawSeries(down,colDown,live&&data.phase==="download");
  drawSeries(up,colUp,live&&data.phase==="upload");

  if(!all.length){
    ctx.textAlign="center";
    ctx.textBaseline="middle";
    ctx.fillStyle=colText;
    ctx.fillText(live?"Waiting for the first throughput sample…":"No throughput samples",padL+cw/2,padT+ch/2);
  }

  function drawRef(v,c){
    if(!(v>0)) return;
    var y=Math.round(Y(v))+0.5;
    ctx.save();
    ctx.setLineDash([4,4]);
    ctx.strokeStyle=alphaHex(c,0.55);
    ctx.beginPath();ctx.moveTo(padL,y);ctx.lineTo(w-padR,y);ctx.stroke();
    ctx.restore();
  }
  function trace(pts){
    ctx.moveTo(X(pts[0].t),Y(pts[0].v));
    for(var k=1;k<pts.length-1;k++){
      var mx=(X(pts[k].t)+X(pts[k+1].t))/2,my=(Y(pts[k].v)+Y(pts[k+1].v))/2;
      ctx.quadraticCurveTo(X(pts[k].t),Y(pts[k].v),mx,my);
    }
    var end=pts[pts.length-1];
    ctx.lineTo(X(end.t),Y(end.v));
  }
  function drawSeries(pts,c,head){
    if(!pts.length) return;
    var last=pts[pts.length-1];
    if(pts.length>1){
      var fill=ctx.createLinearGradient(0,padT,0,padT+ch);
      fill.addColorStop(0,alphaHex(c,0.30));
      fill.addColorStop(1,alphaHex(c,0));
      ctx.beginPath();trace(pts);
      ctx.lineTo(X(last.t),Y(0));ctx.lineTo(X(pts[0].t),Y(0));ctx.closePath();
      ctx.fillStyle=fill;
      ctx.fill();
      ctx.beginPath();trace(pts);
      ctx.strokeStyle=c;
      ctx.lineWidth=2;
      ctx.lineJoin="round";
      ctx.lineCap="round";
      ctx.stroke();
    }
    if(head){
      ctx.save();
      ctx.shadowColor=c;
      ctx.shadowBlur=12;
      ctx.fillStyle=c;
      ctx.beginPath();ctx.arc(X(last.t),Y(last.v),compact?3.5:4,0,Math.PI*2);ctx.fill();
      ctx.restore();
    }
  }
}

/* niceSpeedMax rounds the chart ceiling up to a value whose quarters
   make readable gridline labels (300, 600, 900, 1200 and so on). */
function niceSpeedMax(v){
  var mult=[1,1.2,1.6,2,2.4,3,4,6,8,10];
  var p=Math.pow(10,Math.floor(Math.log(v)/Math.LN10));
  for(var i=0;i<mult.length;i++) if(mult[i]*p>=v) return mult[i]*p;
  return 10*p;
}

function alphaHex(hex,a){
  var m=/^#([0-9a-f]{6})$/i.exec(hex);
  if(!m) return hex;
  var n=parseInt(m[1],16);
  return "rgba("+(n>>16)+","+((n>>8)&255)+","+(n&255)+","+a+")";
}

/* speedSampleTime turns a sample's RFC 3339 timestamp into milliseconds,
   so the chart places each sample where the engine measured it even
   when samples arrive in a burst (#348). The server sends nanoseconds;
   trimming to milliseconds keeps Date.parse happy everywhere. NaN when
   the timestamp is missing or unreadable. */
function speedSampleTime(ts){
  return (typeof ts==="string")?Date.parse(ts.replace(/(\.\d{3})\d+/,"$1")):NaN;
}

/* ── NasSpeedLive ────────────────────────────────────────────────────
   The live speed-test panel shared by the Settings Test button (#346)
   and the dashboard card: a Latency/Download/Upload stepper, live
   readouts, drawSpeedTest's chart and a result area.

   The panel keeps the whole run in JS and fills the element with the
   given id from that state, rebuilding its markup when the element is
   empty. A page that re-renders its HTML mid-test (the dashboard) only
   has to call redraw() afterwards.

   It injects its own stylesheet because the dashboard themes don't
   load /css/shared.css. Colours resolve from whichever page variables
   exist (dashboard --text-primary, settings --text), and data-tone
   switches the series to darker shades on a light background.

   NasSpeedLive.create(opts) -> panel
     opts: {id, compact, stopLabel, closeLabel, onStop, onClose}
     panel.start({startedAt, contractedDown, contractedUp})
     panel.phase(name), panel.sample(sseSample)
     panel.finish({outcome: "complete"|"stopped"|"failed", seconds,
                   download, upload, latency})
     panel.result({badge: {status, label}, summary, lines: [{tone, text}]})
     panel.setStop(enabled, label), panel.hide(), panel.redraw()
     panel.isOpen(), panel.isRunning() */
var SPEED_PHASES=["latency","download","upload"];
var SPEED_PHASE_LABELS={latency:"Measuring latency",download:"Measuring download",upload:"Measuring upload"};
var SPEED_LIVE_CSS=[
  ".speed-live{--sl-down:#60a5fa;--sl-up:#a78bfa;--sl-lat:#4ade80;--sl-text:var(--text-primary,var(--text,#f7f8f8));--sl-muted:var(--text-tertiary,var(--text2,#8a8f98));--sl-faint:var(--text-quaternary,var(--text3,#62666d));--sl-line:var(--border,rgba(127,127,127,0.2));margin-top:14px;padding:14px 16px 12px;border:1px solid var(--sl-line);border-radius:var(--radius,8px);background:var(--elevated,var(--bg-elevated,transparent));color:var(--sl-text);animation:speed-live-open .22s ease-out}",
  ".speed-live[data-tone=light]{--sl-down:#2563eb;--sl-up:#7c3aed;--sl-lat:#16a34a}",
  ".speed-live[hidden],.speed-live [hidden]{display:none}",
  "@keyframes speed-live-open{from{opacity:0;transform:translateY(-4px)}to{opacity:1;transform:none}}",
  "@media (prefers-reduced-motion:reduce){.speed-live{animation:none}}",
  ".speed-live-head{display:flex;align-items:center;gap:10px;flex-wrap:wrap}",
  ".speed-live-steps{display:flex;gap:6px}",
  ".speed-live-step{padding:3px 10px;border:1px solid var(--sl-line);border-radius:999px;font-size:11px;font-weight:600;letter-spacing:.04em;text-transform:uppercase;white-space:nowrap;color:var(--sl-faint);transition:color .2s,border-color .2s}",
  ".speed-live-step[data-state=active][data-step=latency]{color:var(--sl-lat);border-color:var(--sl-lat)}",
  ".speed-live-step[data-state=active][data-step=download]{color:var(--sl-down);border-color:var(--sl-down)}",
  ".speed-live-step[data-state=active][data-step=upload]{color:var(--sl-up);border-color:var(--sl-up)}",
  ".speed-live-step[data-state=done]{color:var(--sl-muted)}",
  ".speed-live-step[data-state=done]::before{content:'✓ '}",
  ".speed-live-actions{display:flex;align-items:center;gap:10px;margin-left:auto}",
  ".speed-live-status{font-size:12px;color:var(--sl-muted);font-variant-numeric:tabular-nums;white-space:nowrap}",
  ".speed-live-stop{padding:4px 10px;border:1px solid var(--sl-line);border-radius:6px;background:transparent;color:var(--sl-muted);font:inherit;font-size:12px;line-height:1.4;cursor:pointer;transition:color .15s,border-color .15s}",
  ".speed-live-stop:hover:not(:disabled){color:var(--sl-text);border-color:var(--sl-muted)}",
  ".speed-live-stop:disabled{cursor:not-allowed;opacity:.6}",
  ".speed-live-readouts{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px;margin:14px 0 8px}",
  ".speed-live-readout{--sl-c:var(--sl-muted);min-width:0}",
  ".speed-live-readout[data-kind=download]{--sl-c:var(--sl-down)}",
  ".speed-live-readout[data-kind=upload]{--sl-c:var(--sl-up)}",
  ".speed-live-readout[data-kind=latency]{--sl-c:var(--sl-lat)}",
  ".speed-live-label{display:flex;align-items:center;gap:6px;font-size:11px;letter-spacing:.05em;text-transform:uppercase;color:var(--sl-muted)}",
  ".speed-live-label::before{content:'';flex:none;width:8px;height:8px;border-radius:50%;background:var(--sl-c)}",
  ".speed-live-num{font-size:28px;font-weight:600;line-height:1.25;color:var(--sl-text);font-variant-numeric:tabular-nums;white-space:nowrap}",
  ".speed-live-num small{margin-left:4px;font-size:12px;font-weight:500;color:var(--sl-muted)}",
  ".speed-live-readout[data-live='1'] .speed-live-num{color:var(--sl-c)}",
  ".speed-live-chart{position:relative;height:190px}",
  ".speed-live-chart canvas{display:block;width:100%;height:100%}",
  ".speed-live-result{display:flex;flex-direction:column;gap:6px;margin-top:10px;padding-top:12px;border-top:1px solid var(--sl-line);font-size:13px;color:var(--sl-text)}",
  ".speed-live-result-head{display:flex;align-items:center;gap:10px;flex-wrap:wrap;font-variant-numeric:tabular-nums}",
  ".speed-live-badge{padding:3px 9px;border-radius:999px;font-size:11px;font-weight:700;letter-spacing:.05em;text-transform:uppercase}",
  ".speed-live-badge[data-status=up]{background:rgba(39,166,68,.15);color:var(--green,#27a644)}",
  ".speed-live-badge[data-status=degraded]{background:rgba(217,119,6,.15);color:var(--amber,#d97706)}",
  ".speed-live-badge[data-status=down]{background:rgba(220,38,38,.15);color:var(--red,#dc2626)}",
  ".speed-live-badge[data-status=stopped]{background:rgba(127,127,127,.15);color:var(--sl-muted)}",
  ".speed-live-meta{font-size:12px;color:var(--sl-muted)}",
  ".speed-live-warn{font-size:12px;color:var(--amber,#d97706)}",
  "@media (max-width:600px){.speed-live-readouts{gap:8px}.speed-live-num{font-size:20px}}",
  /* compact: the dashboard card, where the panel sits inside the card's own box */
  ".speed-live-compact{margin:0 0 12px;padding:0;border:0;border-radius:0;background:none}",
  ".speed-live-compact .speed-live-head{gap:8px}",
  ".speed-live-compact .speed-live-steps{gap:4px}",
  ".speed-live-compact .speed-live-step{padding:2px 6px;font-size:10px;letter-spacing:.02em}",
  ".speed-live-compact .speed-live-actions{gap:8px}",
  ".speed-live-compact .speed-live-status{font-size:11px}",
  ".speed-live-compact .speed-live-stop{padding:3px 9px;font-size:11px}",
  ".speed-live-compact .speed-live-readouts{gap:8px;margin:10px 0 6px}",
  ".speed-live-compact .speed-live-label{font-size:10px}",
  ".speed-live-compact .speed-live-num{font-size:20px}",
  ".speed-live-compact .speed-live-num small{font-size:11px}",
  ".speed-live-compact .speed-live-chart{height:120px}",
  ".speed-live-compact .speed-live-result{margin-top:4px;padding-top:0;border-top:0;font-size:12px}"
].join("\n");

function injectSpeedLiveCSS(){
  if(document.getElementById("speed-live-css")) return;
  var el=document.createElement("style");
  el.id="speed-live-css";
  el.textContent=SPEED_LIVE_CSS;
  (document.head||document.documentElement).appendChild(el);
}

/* surfaceIsLight reports whether the first opaque background behind el
   is light, so the panel can pick series colours that read on it. */
function surfaceIsLight(el){
  for(var n=el;n&&n.nodeType===1;n=n.parentNode){
    var m=/rgba?\((\d+)[,\s]+(\d+)[,\s]+(\d+)(?:[,\s\/]+([\d.]+))?/.exec(getComputedStyle(n).backgroundColor||"");
    if(m&&(m[4]===undefined||parseFloat(m[4])>0.5)){
      return 0.299*m[1]+0.587*m[2]+0.114*m[3]>150;
    }
  }
  return false;
}

function createSpeedLive(opts){
  opts=opts||{};
  var run=null,ticker=0,sizeWatch=null,watched=null;

  function root(){return document.getElementById(opts.id);}
  function part(r,role){return r.querySelector('[data-role="'+role+'"]');}

  function readoutHTML(kind,label,unit){
    return '<div class="speed-live-readout" data-kind="'+kind+'"><div class="speed-live-label">'+label+'</div>'+
      '<div class="speed-live-num"><span data-role="'+kind+'">–</span><small>'+unit+'</small></div></div>';
  }
  function skeleton(){
    return '<div class="speed-live-head"><div class="speed-live-steps">'+
      '<span class="speed-live-step" data-step="latency">Latency</span>'+
      '<span class="speed-live-step" data-step="download">Download</span>'+
      '<span class="speed-live-step" data-step="upload">Upload</span></div>'+
      '<div class="speed-live-actions"><span class="speed-live-status" data-role="status"></span>'+
      '<button type="button" class="speed-live-stop" data-role="stop"></button></div></div>'+
      '<div class="speed-live-readouts">'+readoutHTML("download","Download","Mbps")+readoutHTML("upload","Upload","Mbps")+readoutHTML("latency","Latency","ms")+'</div>'+
      '<div class="speed-live-chart"><canvas data-role="chart" role="img" aria-label="Download and upload speed over time"></canvas></div>'+
      '<div class="speed-live-result" data-role="result" hidden></div>';
  }

  /* elapsed uses the server's start time when the stream sent one, so
     a panel that attaches mid-test shows the test's real age. lag, the
     smallest gap seen between a sample's server timestamp and its
     arrival, absorbs any clock difference between NAS and browser. */
  function elapsed(){
    if(!run) return 0;
    if(run.serverStart&&run.lag!==null) return Math.max(0,(Date.now()-run.lag-run.serverStart)/1000);
    return (Date.now()-run.started)/1000;
  }
  function statusText(){
    var secs=(run.running?elapsed():run.seconds).toFixed(1)+" s";
    if(run.running){
      if(!run.phase) return "Connecting…";
      return opts.compact?secs:SPEED_PHASE_LABELS[run.phase]+" · "+secs;
    }
    if(run.outcome==="complete") return (opts.compact?"Done in ":"Finished in ")+secs;
    if(run.outcome==="stopped") return opts.compact?"Stopped":"Stopped after "+secs;
    return "Failed";
  }
  function fmt(v,digits){return (typeof v==="number"&&isFinite(v)&&v>=0)?v.toFixed(digits):"–";}

  function renderSteps(r){
    var idx=run.outcome==="complete"?SPEED_PHASES.length:SPEED_PHASES.indexOf(run.phase);
    var steps=r.querySelectorAll(".speed-live-step");
    for(var i=0;i<steps.length;i++){
      var k=SPEED_PHASES.indexOf(steps[i].getAttribute("data-step"));
      var state="";
      if(k<idx) state="done";
      else if(k===idx&&run.running) state="active";
      steps[i].setAttribute("data-state",state);
    }
    var readouts=r.querySelectorAll(".speed-live-readout");
    for(var j=0;j<readouts.length;j++){
      var live=run.running&&readouts[j].getAttribute("data-kind")===run.phase;
      readouts[j].setAttribute("data-live",live?"1":"0");
    }
  }
  function renderValues(r){
    var digits={download:0,upload:0,latency:1};
    for(var kind in digits){
      var el=part(r,kind);
      if(el) el.textContent=fmt(run.values[kind],digits[kind]);
    }
  }
  function renderStatus(r){
    var el=part(r,"status");
    if(el) el.textContent=statusText();
  }
  function renderStop(r){
    var b=part(r,"stop");
    if(!b) return;
    if(run.running){
      b.hidden=false;b.disabled=!run.stopEnabled;b.textContent=run.stopLabel;
    }else if(opts.onClose){
      b.hidden=false;b.disabled=false;b.textContent=opts.closeLabel||"Close";
    }else{
      b.hidden=true;
    }
  }
  /* Text nodes only: the server name and ISP come from the engine. */
  function renderResult(r){
    var box=part(r,"result");
    if(!box) return;
    box.textContent="";
    var res=run.result;
    if(res&&(res.badge||res.summary)){
      var head=document.createElement("div");
      head.className="speed-live-result-head";
      if(res.badge){
        var badge=document.createElement("span");
        badge.className="speed-live-badge";
        badge.setAttribute("data-status",res.badge.status);
        badge.textContent=res.badge.label;
        head.appendChild(badge);
      }
      if(res.summary){
        var sum=document.createElement("span");
        sum.textContent=res.summary;
        head.appendChild(sum);
      }
      box.appendChild(head);
    }
    var lines=(res&&res.lines)||[];
    for(var i=0;i<lines.length;i++){
      if(!lines[i]||!lines[i].text) continue;
      var line=document.createElement("div");
      line.className=lines[i].tone==="warn"?"speed-live-warn":"speed-live-meta";
      line.textContent=lines[i].text;
      box.appendChild(line);
    }
    box.hidden=!box.firstChild;
  }

  function draw(){
    var r=root();
    if(!r||!run||r.hidden) return;
    var cs=getComputedStyle(r);
    function cssVar(name,fallback){var v=cs.getPropertyValue(name).trim();return v||fallback;}
    drawSpeedTest(part(r,"chart"),run,{
      live:run.running,
      compact:!!opts.compact,
      font:cs.fontFamily,
      colors:{down:cssVar("--sl-down","#60a5fa"),up:cssVar("--sl-up","#a78bfa"),grid:cssVar("--sl-line","rgba(127,127,127,0.2)"),text:cssVar("--sl-faint","#62666d")}
    });
  }
  /* The chart also has to redraw when its box changes size without a
     window resize: a dashboard re-layout, or the card dragged to a
     column of another width. */
  function watchSize(r){
    if(typeof ResizeObserver==="undefined") return;
    var wrap=r.querySelector(".speed-live-chart");
    if(!wrap||wrap===watched) return;
    if(!sizeWatch) sizeWatch=new ResizeObserver(function(){if(run) scheduleDraw();});
    if(watched) sizeWatch.unobserve(watched);
    sizeWatch.observe(wrap);
    watched=wrap;
  }
  function scheduleDraw(){
    if(!run||run.frame) return;
    if(typeof requestAnimationFrame==="undefined"){draw();return;}
    var mine=run;
    mine.frame=requestAnimationFrame(function(){mine.frame=0;if(run===mine) draw();});
  }
  function render(){
    var r=root();
    if(!r) return;
    if(!run){r.hidden=true;return;}
    injectSpeedLiveCSS();
    if(!part(r,"chart")) r.innerHTML=skeleton();
    watchSize(r);
    r.classList.add("speed-live");
    if(opts.compact) r.classList.add("speed-live-compact");
    r.hidden=false;
    r.setAttribute("data-tone",surfaceIsLight(r)?"light":"dark");
    renderSteps(r);
    renderValues(r);
    renderStatus(r);
    renderStop(r);
    renderResult(r);
    draw();
  }
  function clearTimers(){
    if(ticker){clearInterval(ticker);ticker=0;}
    if(run&&run.frame&&typeof cancelAnimationFrame!=="undefined") cancelAnimationFrame(run.frame);
    if(run) run.frame=0;
  }

  /* settle runs when a phase ends. A finished latency phase shows its
     average ping, which is what the engine reports. Throughput keeps
     the phase's last sample: speedtest-go's samples are its moving
     average (EWMA) and its result is that average's final value, so
     averaging the samples again reads low because it counts the ramp-up
     (786 Mbps on screen against an 872 result in a real run). The Ookla
     CLI fallback sends no live samples. */
  function settle(phase){
    if(phase!=="latency"||!run.lat.length) return;
    var sum=0;
    for(var i=0;i<run.lat.length;i++) sum+=run.lat[i];
    run.values.latency=sum/run.lat.length;
  }

  function start(cfg){
    cfg=cfg||{};
    clearTimers();
    var serverStart=speedSampleTime(cfg.startedAt);
    run={
      started:Date.now(),serverStart:isFinite(serverStart)?serverStart:0,lag:null,
      t0:0,phase:"",lat:[],down:[],up:[],
      contractedDown:Number(cfg.contractedDown)||0,contractedUp:Number(cfg.contractedUp)||0,
      values:{download:null,upload:null,latency:null},
      running:true,outcome:"",seconds:0,
      stopEnabled:true,stopLabel:opts.stopLabel||"Stop",
      result:null,frame:0
    };
    ticker=setInterval(function(){var r=root();if(r&&run&&run.running) renderStatus(r);},500);
    render();
  }
  function phase(name){
    if(!run||!run.running) return;
    settle(run.phase);
    run.phase=name||"";
    var r=root();
    if(r){renderSteps(r);renderValues(r);renderStatus(r);}
  }
  function sample(d){
    if(!run||!run.running||!d) return;
    var at=speedSampleTime(d.ts);
    if(isFinite(at)){
      var lag=Date.now()-at;
      if(run.lag===null||lag<run.lag) run.lag=lag;
    }else{
      at=Date.now();
    }
    if(d.phase==="latency"){
      if(typeof d.latency_ms==="number"&&d.latency_ms>0){
        run.lat.push(d.latency_ms);
        run.values.latency=d.latency_ms;
      }
    }else if(typeof d.mbps==="number"){
      if(!run.t0) run.t0=at;
      var point={t:Math.max(0,(at-run.t0)/1000),v:d.mbps};
      if(d.phase==="upload"){run.up.push(point);run.values.upload=d.mbps;}
      else{run.down.push(point);run.values.download=d.mbps;}
      scheduleDraw();
    }
    var r=root();
    if(r){renderValues(r);renderStatus(r);}
  }
  function finish(f){
    if(!run||!run.running) return;
    f=f||{};
    var secs=f.seconds>0?f.seconds:elapsed();
    clearTimers();
    run.running=false;
    run.seconds=secs;
    run.outcome=f.outcome||"complete";
    if(run.outcome==="complete"){
      settle(run.phase);
      if(typeof f.download==="number") run.values.download=f.download;
      if(typeof f.upload==="number") run.values.upload=f.upload;
      if(typeof f.latency==="number") run.values.latency=f.latency;
    }
    render();
  }
  function setStop(enabled,label){
    if(!run) return;
    run.stopEnabled=!!enabled;
    if(typeof label==="string") run.stopLabel=label;
    var r=root();
    if(r) renderStop(r);
  }
  function setResult(res){
    if(!run) return;
    run.result=res||null;
    var r=root();
    if(r) renderResult(r);
  }
  function hide(){
    clearTimers();
    run=null;
    render();
  }

  if(opts.onStop||opts.onClose){
    document.addEventListener("click",function(e){
      var t=e.target,r=root();
      if(!r||!t||!t.closest) return;
      var b=t.closest('[data-role="stop"]');
      if(!b||!r.contains(b)) return;
      if(run&&run.running){if(opts.onStop) opts.onStop();}
      else if(opts.onClose) opts.onClose();
    });
  }
  window.addEventListener("resize",function(){if(run) scheduleDraw();});

  return {
    start:start,phase:phase,sample:sample,finish:finish,
    setStop:setStop,result:setResult,hide:hide,redraw:render,
    isOpen:function(){return !!run;},
    isRunning:function(){return !!run&&run.running;}
  };
}

/* ── public API ──────────────────────────────────────────────────── */
var NasChart={
  line:      drawLine,
  area:      drawArea,
  bar:       drawBar,
  gauge:     drawGauge,
  sparkline: drawSparkline,
  speedTest: drawSpeedTest,
  /* _decimateLabels is exposed for unit tests only. It is not part of
     the public API — name is prefixed with an underscore to signal
     "internal / subject to change". See issue #165 and
     internal/api/charts_decimation_test.go. */
  _decimateLabels: decimateLabels
};

window.NasChart=NasChart;
window.NasSpeedLive={create:createSpeedLive};
})();

/* ================================================================
   NasDrag — Pointer-based section reorder for dashboard columns.
   Sections lift and follow the cursor; others animate to fill the gap.
   ================================================================ */
(function(){
"use strict";

var LAYOUT_KEY = "nas-doctor-dashboard-order";
var dragging = null;
var columns = [];

function init() {
  // Support N columns: col-left, col-right, col-3, col-4, ...
  columns = [];
  var cl = document.getElementById("col-left");
  var cr = document.getElementById("col-right");
  if (cl) columns.push(cl);
  if (cr) columns.push(cr);
  for (var ci = 3; ci <= 6; ci++) {
    var extra = document.getElementById("col-" + ci);
    if (extra) columns.push(extra);
  }
  if (columns.length === 0) return;

  var sections = document.querySelectorAll(".section-block[data-section]");
  var gripSVG = '<svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor" style="pointer-events:none"><circle cx="9" cy="6" r="1.5"/><circle cx="15" cy="6" r="1.5"/><circle cx="9" cy="12" r="1.5"/><circle cx="15" cy="12" r="1.5"/><circle cx="9" cy="18" r="1.5"/><circle cx="15" cy="18" r="1.5"/></svg>';

  for (var i = 0; i < sections.length; i++) {
    var sec = sections[i];
    if (sec.querySelector(".section-drag-handle")) continue;
    var title = sec.querySelector(".section-title");
    if (!title) continue;
    if (!title.parentElement.classList.contains("section-title-row")) {
      var row = document.createElement("div");
      row.className = "section-title-row";
      var handle = document.createElement("div");
      handle.className = "section-drag-handle";
      handle.innerHTML = gripSVG;
      title.parentNode.insertBefore(row, title);
      row.appendChild(handle);
      row.appendChild(title);
    }
  }

  document.addEventListener("mousedown", onDown, false);
  document.addEventListener("touchstart", onDown, { passive: false });
}

function onDown(e) {
  var handle = (e.target.closest ? e.target.closest(".section-drag-handle") : null);
  if (!handle) return;
  var sec = handle.closest(".section-block[data-section]");
  if (!sec) return;

  e.preventDefault();
  var pt = getPoint(e);
  var rect = sec.getBoundingClientRect();

  // Create placeholder
  var ph = document.createElement("div");
  ph.className = "section-drop-placeholder";
  ph.style.height = rect.height + "px";
  sec.parentNode.insertBefore(ph, sec);

  // Lift section
  sec.style.position = "fixed";
  sec.style.zIndex = "9000";
  sec.style.width = rect.width + "px";
  sec.style.left = rect.left + "px";
  sec.style.top = rect.top + "px";
  sec.style.transition = "none";
  sec.style.boxShadow = "0 12px 40px rgba(0,0,0,0.3)";
  sec.style.opacity = "0.92";
  sec.style.pointerEvents = "none";
  sec.classList.add("dragging");
  document.body.appendChild(sec);

  dragging = {
    el: sec,
    placeholder: ph,
    offsetX: pt.x - rect.left,
    offsetY: pt.y - rect.top,
    width: rect.width,
    height: rect.height
  };

  document.addEventListener("mousemove", onMove, false);
  document.addEventListener("touchmove", onMove, { passive: false });
  document.addEventListener("mouseup", onUp, false);
  document.addEventListener("touchend", onUp, false);
}

function onMove(e) {
  if (!dragging) return;
  e.preventDefault();
  var pt = getPoint(e);

  // Move the element with cursor
  dragging.el.style.left = (pt.x - dragging.offsetX) + "px";
  dragging.el.style.top = (pt.y - dragging.offsetY) + "px";

  // Find which column the cursor is over
  var targetCol = null;
  for (var c = 0; c < columns.length; c++) {
    var cr = columns[c].getBoundingClientRect();
    if (pt.x >= cr.left && pt.x <= cr.right) {
      targetCol = columns[c];
      break;
    }
  }
  if (!targetCol) return;

  // Move placeholder to new position
  var afterEl = getInsertAfter(targetCol, pt.y);
  if (dragging.placeholder.parentNode !== targetCol || dragging.placeholder.nextElementSibling !== afterEl) {
    if (afterEl) {
      targetCol.insertBefore(dragging.placeholder, afterEl);
    } else {
      targetCol.appendChild(dragging.placeholder);
    }
  }
}

function onUp(e) {
  if (!dragging) return;

  var el = dragging.el;
  var ph = dragging.placeholder;

  // Animate to placeholder position
  var phRect = ph.getBoundingClientRect();
  el.style.transition = "left 0.2s ease, top 0.2s ease, opacity 0.2s ease, box-shadow 0.2s ease";
  el.style.left = phRect.left + "px";
  el.style.top = phRect.top + "px";
  el.style.opacity = "1";
  el.style.boxShadow = "none";

  setTimeout(function() {
    // Re-insert in DOM at placeholder position
    el.style.position = "";
    el.style.zIndex = "";
    el.style.width = "";
    el.style.left = "";
    el.style.top = "";
    el.style.transition = "";
    el.style.boxShadow = "";
    el.style.opacity = "";
    el.style.pointerEvents = "";
    el.classList.remove("dragging");

    if (ph.parentNode) {
      ph.parentNode.insertBefore(el, ph);
      ph.remove();
    }

    saveOrder();
  }, 220);

  dragging = null;
  document.removeEventListener("mousemove", onMove, false);
  document.removeEventListener("touchmove", onMove, false);
  document.removeEventListener("mouseup", onUp, false);
  document.removeEventListener("touchend", onUp, false);
}

function getInsertAfter(col, y) {
  var children = col.querySelectorAll(".section-block[data-section], .section-drop-placeholder");
  var closest = null;
  var closestDist = Number.NEGATIVE_INFINITY;
  for (var i = 0; i < children.length; i++) {
    var child = children[i];
    if (child.classList.contains("dragging")) continue;
    var box = child.getBoundingClientRect();
    var mid = box.top + box.height / 2;
    var dist = y - mid;
    if (dist < 0 && dist > closestDist) {
      closestDist = dist;
      closest = child;
    }
  }
  return closest;
}

function getPoint(e) {
  if (e.touches && e.touches.length) return { x: e.touches[0].clientX, y: e.touches[0].clientY };
  return { x: e.clientX, y: e.clientY };
}

function getSavedOrder() {
  try { var r = localStorage.getItem(LAYOUT_KEY); return r ? JSON.parse(r) : null; } catch(e) { return null; }
}

function saveOrder() {
  // Build order: { "col-0": ["findings","docker"], "col-1": ["drives","gpu"], ... }
  var order = {};
  for (var c = 0; c < columns.length; c++) {
    var arr = [];
    var bs = columns[c].querySelectorAll(".section-block[data-section]");
    for (var i = 0; i < bs.length; i++) arr.push(bs[i].getAttribute("data-section"));
    order["col-" + c] = arr;
  }
  // Save to server (persists across updates/reboots)
  fetch("/api/v1/settings/section-order", { method: "PUT", headers: {"Content-Type":"application/json"}, body: JSON.stringify(order) }).catch(function(){});
  // Also save to localStorage as immediate cache
  try { localStorage.setItem(LAYOUT_KEY, JSON.stringify(order)); } catch(e) {}
}

function applySavedOrder(blockMap, visibleItems, allCols) {
  // Try server-saved order first (from statusData.section_order), then localStorage
  var saved = null;
  if (window._serverSectionOrder) saved = window._serverSectionOrder;
  if (!saved) saved = getSavedOrder();
  if (!saved) return false;

  // Normalize: server sends {"col-0":[...],"col-1":[...]}, localStorage may have {cols:[...]} or {left:[...],right:[...]}
  var colArrays = [];
  if (saved["col-0"]) {
    for (var i = 0; i < allCols.length; i++) {
      colArrays.push(saved["col-" + i] || []);
    }
  } else if (saved.cols) {
    colArrays = saved.cols;
  } else if (saved.left && saved.right) {
    colArrays = [saved.left, saved.right];
  }
  if (colArrays.length === 0) return false;

  var used = {};
  for (var c = 0; c < Math.min(colArrays.length, allCols.length); c++) {
    var colOrder = colArrays[c] || [];
    for (var s = 0; s < colOrder.length; s++) {
      var name = colOrder[s];
      if (blockMap[name] && !used[name]) {
        allCols[c].appendChild(blockMap[name]);
        used[name] = true;
      }
    }
  }
  // Distribute remaining sections not in saved order
  for (var k = 0; k < visibleItems.length; k++) {
    if (!used[visibleItems[k].name]) {
      var minIdx = 0, minH = allCols[0].offsetHeight;
      for (var m = 1; m < allCols.length; m++) {
        if (allCols[m].offsetHeight < minH) { minH = allCols[m].offsetHeight; minIdx = m; }
      }
      allCols[minIdx].appendChild(visibleItems[k].el);
    }
  }
  return true;
}

window.NasDrag = {
  init: init,
  getSavedOrder: getSavedOrder,
  applySavedOrder: applySavedOrder
};
})();

/* ================================================================
   NasSwipe — Swipe-to-dismiss for finding cards (touch + mouse).
   Swipe left to reveal dismiss action; release past threshold to dismiss.
   ================================================================ */
(function(){
"use strict";

var THRESHOLD = 0.3;
var active = null;

function init() {
  var list = document.querySelector(".findings-list");
  if (!list) return;
  list.addEventListener("touchstart", onStart, { passive: true });
  list.addEventListener("mousedown", onStart, false);
}

function onStart(e) {
  var finding = e.target.closest(".finding");
  if (!finding) return;
  if (e.target.closest("a") || e.target.closest("button")) return;

  var pt = getPoint(e);
  var rect = finding.getBoundingClientRect();

  active = { el: finding, startX: pt.x, startY: pt.y, width: rect.width, dismissed: false, locked: false };

  if (!finding.querySelector(".swipe-dismiss-bg")) {
    var bg = document.createElement("div");
    bg.className = "swipe-dismiss-bg";
    bg.textContent = "Dismiss";
    finding.appendChild(bg);
  }
  finding.style.transition = "none";

  document.addEventListener("touchmove", onMove, { passive: false });
  document.addEventListener("mousemove", onMove, false);
  document.addEventListener("touchend", onEnd, false);
  document.addEventListener("mouseup", onEnd, false);
}

function onMove(e) {
  if (!active) return;
  var pt = getPoint(e);
  var dx = pt.x - active.startX;
  var dy = pt.y - active.startY;

  if (!active.locked && Math.abs(dy) > Math.abs(dx) && Math.abs(dy) > 8) {
    resetCard(); cleanup(); return;
  }
  active.locked = true;
  if (dx > 0) dx = 0;
  if (dx === 0) return;
  e.preventDefault();

  var pct = Math.abs(dx) / active.width;
  active.el.style.transform = "translateX(" + dx + "px)";
  var bg = active.el.querySelector(".swipe-dismiss-bg");
  if (bg) bg.style.opacity = String(Math.min(1, pct / THRESHOLD));
  active.dismissed = pct >= THRESHOLD;
}

function onEnd() {
  if (!active) return;
  var el = active.el;

  if (active.dismissed) {
    el.style.transition = "transform 0.25s ease, opacity 0.25s ease";
    el.style.transform = "translateX(-100%)";
    el.style.opacity = "0";
    var title = "";
    var titleEl = el.querySelector(".finding-title");
    if (titleEl) title = titleEl.textContent;
    setTimeout(function() {
      el.style.height = el.offsetHeight + "px";
      el.offsetHeight; /* force reflow */
      el.style.transition = "height 0.2s ease, margin 0.2s ease, padding 0.2s ease, opacity 0.2s ease";
      el.style.height = "0"; el.style.marginBottom = "0"; el.style.paddingTop = "0"; el.style.paddingBottom = "0";
      setTimeout(function() { el.remove(); }, 220);
      if (title && window._dismissFinding) window._dismissFinding(title, true);
    }, 260);
  } else {
    resetCard();
  }
  cleanup();
}

function resetCard() {
  if (!active) return;
  active.el.style.transition = "transform 0.2s ease";
  active.el.style.transform = "";
  var bg = active.el.querySelector(".swipe-dismiss-bg");
  if (bg) bg.style.opacity = "0";
}

function cleanup() {
  document.removeEventListener("touchmove", onMove, false);
  document.removeEventListener("mousemove", onMove, false);
  document.removeEventListener("touchend", onEnd, false);
  document.removeEventListener("mouseup", onEnd, false);
  active = null;
}

function getPoint(e) {
  if (e.touches && e.touches.length) return { x: e.touches[0].clientX, y: e.touches[0].clientY };
  return { x: e.clientX, y: e.clientY };
}

window.NasSwipe = { init: init };
})();

/* ================================================================
   NasSort — Sort controls for findings and drives.
   Renders a compact pill-bar of sort options that re-sort DOM elements.
   ================================================================ */
(function(){
"use strict";

var SEV_ORDER = { critical: 0, warning: 1, info: 2, ok: 3 };
var SORT_KEY = "nas-doctor-sort-prefs";

function getPrefs() {
  try { var r = localStorage.getItem(SORT_KEY); return r ? JSON.parse(r) : {}; } catch(e) { return {}; }
}
function savePrefs(p) {
  try { localStorage.setItem(SORT_KEY, JSON.stringify(p)); } catch(e) {}
}

/* Parse sort key — "severity" or "severity-rev" */
function parseSort(s) {
  if (!s) return { key: "", rev: false };
  if (s.indexOf("-rev") === s.length - 4) return { key: s.slice(0, -4), rev: true };
  return { key: s, rev: false };
}

/* Render a pill-bar sort control. active can be "key" or "key-rev". */
function renderSortBar(opts) {
  if (opts.container) opts.container.innerHTML = "";
  var bar = document.createElement("div");
  bar.className = "sort-bar";
  var parsed = parseSort(opts.active);
  for (var i = 0; i < opts.options.length; i++) {
    var o = opts.options[i];
    var isActive = o.key === parsed.key;
    var pill = document.createElement("button");
    pill.className = "sort-pill" + (isActive ? " active" : "");
    var arrow = isActive ? (parsed.rev ? " \u2191" : " \u2193") : "";
    pill.textContent = o.label + arrow;
    pill.setAttribute("data-sort-key", o.key);
    pill.onclick = (function(key) { return function() {
      var cur = parseSort(opts.active);
      var next = (cur.key === key && !cur.rev) ? key + "-rev" : key;
      opts.onSort(next);
    }; })(o.key);
    bar.appendChild(pill);
  }
  if (opts.container) opts.container.appendChild(bar);
  return bar;
}

/* Sort findings array in place. key can be "severity", "severity-rev", etc. */
function sortFindings(findings, sortKey) {
  var p = parseSort(sortKey);
  var dir = p.rev ? -1 : 1;
  if (p.key === "severity") {
    findings.sort(function(a, b) { return dir * ((SEV_ORDER[a.severity] || 9) - (SEV_ORDER[b.severity] || 9)); });
  } else if (p.key === "date") {
    findings.sort(function(a, b) {
      var da = a.detected_at ? new Date(a.detected_at).getTime() : 0;
      var db = b.detected_at ? new Date(b.detected_at).getTime() : 0;
      return dir * (db - da);
    });
  } else if (p.key === "category") {
    findings.sort(function(a, b) {
      var ca = (a.category || "").toLowerCase();
      var cb = (b.category || "").toLowerCase();
      return dir * (ca < cb ? -1 : ca > cb ? 1 : 0);
    });
  }
  return findings;
}

/* Sort SMART drives array in place. */
function sortDrives(drives, sortKey) {
  var p = parseSort(sortKey);
  var dir = p.rev ? -1 : 1;
  if (p.key === "health") {
    drives.sort(function(a, b) { return dir * ((a.health_passed ? 1 : 0) - (b.health_passed ? 1 : 0)); });
  } else if (p.key === "temp") {
    drives.sort(function(a, b) { return dir * ((b.temperature_c || 0) - (a.temperature_c || 0)); });
  } else if (p.key === "age") {
    drives.sort(function(a, b) { return dir * ((b.power_on_hours || 0) - (a.power_on_hours || 0)); });
  } else if (p.key === "size") {
    drives.sort(function(a, b) { return dir * ((b.size_gb || 0) - (a.size_gb || 0)); });
  } else if (p.key === "device") {
    drives.sort(function(a, b) { return dir * ((a.device || "").localeCompare(b.device || "")); });
  }
  return drives;
}

/* Sort storage disks. */
function sortStorage(disks, key) {
  if (key === "usage") {
    disks.sort(function(a, b) { return (b.used_percent || 0) - (a.used_percent || 0); }); // fullest first
  } else if (key === "free") {
    disks.sort(function(a, b) { return (a.free_gb || 0) - (b.free_gb || 0); }); // least free first
  } else if (key === "size") {
    disks.sort(function(a, b) { return (b.total_gb || 0) - (a.total_gb || 0); }); // largest first
  } else if (key === "name") {
    disks.sort(function(a, b) {
      var na = (a.label || a.mount_point || "").toLowerCase();
      var nb = (b.label || b.mount_point || "").toLowerCase();
      return na < nb ? -1 : na > nb ? 1 : 0;
    });
  }
  return disks;
}

window.NasSort = {
  renderSortBar: renderSortBar,
  sortFindings: sortFindings,
  sortDrives: sortDrives,
  sortStorage: sortStorage,
  getPrefs: getPrefs,
  savePrefs: savePrefs,
  SEV_ORDER: SEV_ORDER
};
})();
`
