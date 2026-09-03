// radar.js — mobile collapse toggles + client-side filter chips. No deps.
(function(){
  var cards=[].slice.call(document.querySelectorAll('.list .card'));
  // collapse toggles
  cards.forEach(function(c){
    var t=c.querySelector('.tog');if(!t)return;
    t.addEventListener('click',function(){var o=c.classList.toggle('open');t.setAttribute('aria-expanded',o);});
  });
  // deep link (#fNN) opens that card
  if(location.hash){var h=document.getElementById(location.hash.slice(1));if(h){h.classList.add('open');h.scrollIntoView({block:'center'});}}
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
