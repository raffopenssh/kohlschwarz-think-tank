// radar.js — mobile collapse toggles + client-side filter chips. No deps.
(function(){
  var cards=[].slice.call(document.querySelectorAll('.list .card'));
  // collapse toggles
  cards.forEach(function(c){
    var t=c.querySelector('.tog');if(!t)return;
    t.addEventListener('click',function(){var o=c.classList.toggle('open');t.setAttribute('aria-expanded',o);});
  });
  // deep link (#fNN) opens that card
  if(location.hash){var h=document.getElementById(location.hash.slice(1));if(h){h.classList.add('open');var d=h.querySelector('details');if(d)d.open=true;h.scrollIntoView({block:'center'});}}
  // filters
  var bar=document.getElementById('filters'),count=document.getElementById('count');if(!bar)return;
  var bs=[].slice.call(bar.querySelectorAll('button'));
  function match(c,f){
    var d=c.dataset;
    switch(f){
      case '':return true;
      case 'new':return d.new==='1';
      case 'top':return +d.score>=70;
      case 'deadline':return !!d.deadline;
      case 'hard':return d.verdict==='hard to fill';
      case 'live':return d.verdict!=='closed'&&d.verdict!=='gone';
      case 'open':return d.status==='open';
      case 'soon':return d.soon==='1'&&d.status!=='skip'&&d.status!=='rejected';
      case 'ssa':case 'at':case 'eu':return (d.region||d.track)===f;
      default:return d.track===f||d.kind===f||d.region===f;
    }
  }
  function apply(f){
    var n=0;cards.forEach(function(c){var m=match(c,f);c.hidden=!m;if(m)n++;});
    bs.forEach(function(b){b.setAttribute('aria-pressed',b.dataset.f===f?'true':'false');});
    if(count)count.textContent=n+'/'+cards.length;
    try{f?history.replaceState(null,'','?f='+f):history.replaceState(null,'',location.pathname+location.hash);}catch(e){}
  }
  bs.forEach(function(b){b.addEventListener('click',function(){apply(b.dataset.f);});});
  var q=new URLSearchParams(location.search).get('f');
  apply(q&&bs.some(function(b){return b.dataset.f===q;})?q:'');
})();

// live "updating" indicator for /admin/jobs: poll status.json while a job runs, reload when done.
(function(){
  var st=document.getElementById('status');if(!st)return;
  var live=document.getElementById('live'),txt=document.getElementById('live-text'),upd=document.getElementById('upd');
  var running=st.dataset.running||'';
  function show(kind,since){
    running=kind;document.body.classList.toggle('busy',!!kind);
    live.hidden=!kind;upd.hidden=!!kind;
    if(kind){var s=since?Math.max(0,Math.round(Date.now()/1000-since)):0;txt.textContent=kind+'ing…'+(s>=5?' '+(s<60?s+'s':Math.round(s/60)+' min'):'');}
  }
  var wasRunning=!!running;
  show(running,+st.dataset.since||0);
  var delay=wasRunning?2000:15000,timer;
  function poll(){
    fetch('/admin/jobs/status.json',{credentials:'same-origin',cache:'no-store'}).then(function(r){return r.json();}).then(function(j){
      if(j.running){wasRunning=true;delay=2000;show(j.running,j.since);}
      else if(wasRunning){location.replace(location.pathname+(location.search||'')+location.hash);return;}
      else{delay=Math.min(delay*2,60000);}
      timer=setTimeout(poll,delay);
    }).catch(function(){timer=setTimeout(poll,10000);});
  }
  timer=setTimeout(poll,delay);
  document.addEventListener('visibilitychange',function(){if(!document.hidden){clearTimeout(timer);poll();}});
  // clicking a bar button: show indicator immediately (before redirect lands)
  var bar=document.getElementById('bar');if(bar)bar.addEventListener('submit',function(e){
    var a=(e.target.getAttribute('action')||'').split('/').pop();show(a,Math.round(Date.now()/1000));
  });
})();
