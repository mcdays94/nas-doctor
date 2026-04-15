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
  var w=opts&&opts.width?opts.width:pw;
  var h=opts&&opts.height?opts.height:el.getAttribute("height")?parseInt(el.getAttribute("height"),10):200;
  var dpr=window.devicePixelRatio||1;
  el.width=w*dpr; el.height=h*dpr;
  el.style.width=w+"px"; el.style.height=h+"px";
  var ctx=el.getContext("2d");
  ctx.scale(dpr,dpr);
  return {el:el,ctx:ctx,w:w,h:h,dpr:dpr};
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
  var tip=document.createElement("div");
  tip.style.cssText="position:fixed;padding:6px 10px;border-radius:6px;font:11px/1.4 -apple-system,system-ui,sans-serif;pointer-events:none;opacity:0;transition:opacity .15s;z-index:9999;max-width:220px;white-space:nowrap;";
  document.body.appendChild(tip);
  var cross={x:-1,active:false};

  el.addEventListener("mousemove",function(e){
    var r=el.getBoundingClientRect();
    var x=e.clientX-r.left, y=e.clientY-r.top;
    var info=hitTest(x,y,cross);
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
    var info=hitTest(-1,-1,cross);
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
  /* x labels */
  if(labels&&labels.length){
    ctx.textAlign="center"; ctx.textBaseline="top";
    var step=Math.max(1,Math.floor(labels.length/(cw/50)));
    for(var j=0;j<labels.length;j+=step){
      var px=m.l+j/(labels.length-1||1)*cw;
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

  animate(500,function(t){render(t);});

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

  animate(500,function(t){render(t);});

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

  animate(500,function(t){render(t);});

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

  animate(500,function(t){render(t);});
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

/* ── public API ──────────────────────────────────────────────────── */
var NasChart={
  line:      drawLine,
  area:      drawArea,
  bar:       drawBar,
  gauge:     drawGauge,
  sparkline: drawSparkline
};

window.NasChart=NasChart;
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
