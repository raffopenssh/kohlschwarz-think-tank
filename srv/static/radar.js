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
      case 'up':return d.vote==='1';
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

// owner feedback: thumbs / note / status / trash without page reloads, plus a
// one-time "why?" prompt after trashing. Falls back to plain forms without JS.
(function(){
  document.documentElement.classList.add('js');
  var reacts=[].slice.call(document.querySelectorAll('.react'));if(!reacts.length)return;
  function post(url,data){
    var b=new URLSearchParams();Object.keys(data||{}).forEach(function(k){b.append(k,data[k]);});
    return fetch(url,{method:'POST',credentials:'same-origin',headers:{'Accept':'application/json','Content-Type':'application/x-www-form-urlencoded'},body:b.toString()})
      .then(function(r){if(!r.ok)throw new Error('HTTP '+r.status);return r.json();});
  }
  var filterCount=function(){var b=document.querySelector('#filters button[aria-pressed=true]');if(b)b.click();};
  reacts.forEach(function(R){
    var card=R.closest('.card'),radar=R.dataset.radar,id=R.dataset.id,saved=R.querySelector('.saved'),t;
    var base='/admin/'+(radar==='job'?'jobs':'funding')+'/';
    function flash(msg,err){saved.textContent=msg;saved.classList.toggle('err',!!err);saved.classList.add('show');clearTimeout(t);t=setTimeout(function(){saved.classList.remove('show');},err?4000:1500);}
    // thumbs
    var vb=[].slice.call(R.querySelectorAll('.vote .ib'));
    function paintVote(v){vb.forEach(function(b){var on=+b.value===v;b.classList.toggle('on',on);b.setAttribute('aria-pressed',on);});
      card.dataset.vote=v;card.classList.toggle('voted-up',v===1);card.classList.toggle('voted-down',v===-1);}
    vb.forEach(function(b){b.addEventListener('click',function(e){e.preventDefault();
      var v=+b.value,cur=+card.dataset.vote||0,nv=v===cur?0:v;paintVote(nv);
      post(base+'vote/'+id,{vote:v}).then(function(j){paintVote(j.vote);flash(j.vote?'saved':'cleared');}).catch(function(){paintVote(cur);flash('failed',1);});
    });});
    // note
    var tog=R.querySelector('.note-tog'),nf=R.querySelector('.note-form'),ta=nf.querySelector('textarea'),last=ta.value,saveT;
    function openNote(o){nf.hidden=!o;tog.setAttribute('aria-expanded',o);if(o){ta.focus();ta.setSelectionRange(ta.value.length,ta.value.length);grow();}}
    function grow(){if(!('fieldSizing' in ta.style)){ta.style.height='auto';ta.style.height=Math.min(ta.scrollHeight+2,14*16)+'px';}}
    function saveNote(){var v=ta.value.trim();if(v===last){nf.classList.remove('dirty');return Promise.resolve();}
      return post(base+'note/'+id,{note:v}).then(function(j){last=j.note;nf.classList.remove('dirty');tog.classList.toggle('has',!!last);tog.title=last?'Edit note':'Add a note';flash('note saved');}).catch(function(){flash('not saved',1);});}
    tog.addEventListener('click',function(){openNote(nf.hidden);});
    ta.addEventListener('input',function(){grow();nf.classList.toggle('dirty',ta.value.trim()!==last);clearTimeout(saveT);saveT=setTimeout(saveNote,1200);});
    ta.addEventListener('blur',function(){clearTimeout(saveT);saveNote();});
    ta.addEventListener('keydown',function(e){
      if((e.metaKey||e.ctrlKey)&&e.key==='Enter'){e.preventDefault();clearTimeout(saveT);saveNote().then(function(){if(!ta.value.trim())openNote(false);});}
      else if(e.key==='Escape'){e.preventDefault();clearTimeout(saveT);saveNote();openNote(!!ta.value.trim()&&false);}
    });
    nf.addEventListener('submit',function(e){e.preventDefault();clearTimeout(saveT);saveNote();});
    // why? (asked once per item)
    var why=R.querySelector('.why-ask');
    function askWhy(undo){
      why.hidden=false;
      var chips=[].slice.call(why.querySelectorAll('.chips button'));
      chips.forEach(function(c){c.onclick=function(){chips.forEach(function(x){x.classList.toggle('on',x===c);});
        var data={reason:c.dataset.reason};if(radar==='grant')data.status=R.querySelector('.stat select').value;
        post(base+(radar==='job'?'hide/':'status/')+id,data).then(function(){card.dataset.asked='1';why.hidden=true;flash('thanks');}).catch(function(){flash('failed',1);});};});
      why.querySelector('.skip').onclick=function(){why.hidden=true;};
    }
    // trash / restore (jobs)
    var tf=R.querySelector('.trash');if(tf){tf.addEventListener('submit',function(e){e.preventDefault();
      var un=!!tf.querySelector('[name=unhide]');card.classList.add('leaving');
      post(base+'hide/'+id,un?{unhide:'1'}:{}).then(function(j){
        card.classList.remove('leaving');card.classList.toggle('hidden-row',j.hidden);card.dataset.hidden=j.hidden?'1':'';
        var showingHidden=/[?&]hidden=1/.test(location.search);
        tf.innerHTML=j.hidden?'<input type="hidden" name="unhide" value="1"><button class="ib txt" title="Put this posting back on the list"><svg class="i"><use href="#i-undo"/></svg><span class="lbl">restore</span></button>'
          :'<button class="ib txt" title="Remove from the list (not relevant)."><svg class="i"><use href="#i-trash"/></svg><span class="lbl">not relevant</span></button>';
        if(j.hidden&&j.ask_reason)askWhy();else why.hidden=true;
        if(j.hidden&&!showingHidden&&!j.ask_reason){setTimeout(function(){card.hidden=true;filterCount();},250);}
        else if(j.hidden&&!showingHidden){why.querySelector('.skip').addEventListener('click',function(){card.hidden=true;filterCount();},{once:true});
          [].forEach.call(why.querySelectorAll('.chips button'),function(c){c.addEventListener('click',function(){setTimeout(function(){card.hidden=true;filterCount();},600);},{once:true});});}
        flash(j.hidden?'removed':'restored');
      }).catch(function(){card.classList.remove('leaving');flash('failed',1);});
    });}
    // status (grants)
    var sf=R.querySelector('.stat');if(sf){var sel=sf.querySelector('select'),prev=sel.value;
      sel.addEventListener('change',function(){var v=sel.value;
        post(base+'status/'+id,{status:v}).then(function(j){prev=j.status;card.dataset.status=j.status;
          card.classList.toggle('done',/^(skip|rejected|won)$/.test(j.status));
          var tg=card.querySelector('.tags .tag.status');if(tg)tg.remove();
          if(j.status!=='open'){var sp=document.createElement('span');sp.className='tag status '+j.status;sp.textContent=j.status;card.querySelector('.tags').appendChild(sp);}
          if(j.ask_reason)askWhy();else why.hidden=true;flash('status: '+j.status);
        }).catch(function(){sel.value=prev;flash('failed',1);});
      });
      sf.addEventListener('submit',function(e){e.preventDefault();});}
  });
})();
