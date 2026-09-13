const qs=s=>document.querySelector(s);
const form=qs("#generator"),statusEl=qs("#status"),result=qs("#result");
let latest;

form.addEventListener("submit",async e=>{
  e.preventDefault();
  statusEl.textContent="جاري إعداد السكربت…";
  result.hidden=true;
  const data=Object.fromEntries(new FormData(form));
  data.duration_seconds=Number(data.duration_seconds);
  try{
    const res=await fetch("/api/script",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(data)});
    if(!res.ok) throw new Error(await res.text());
    latest=await res.json();
    qs("#hook").textContent=latest.recommended_hook;
    qs("#caption").textContent=latest.caption;
    qs("#hashtags").textContent=latest.hashtags.join(" ");
    qs("#result tbody").replaceChildren(...latest.scenes.map(s=>{
      const tr=document.createElement("tr");
      [s.seconds,s.shot,s.voiceover,s.on_screen].forEach(v=>{const td=document.createElement("td");td.textContent=v;tr.append(td)});
      return tr;
    }));
    result.hidden=false;
    statusEl.textContent="";
  }catch(err){statusEl.textContent=err.message}
});

qs("#download").addEventListener("click",()=>{
  if(!latest)return;
  const a=document.createElement("a");
  a.href=URL.createObjectURL(new Blob([JSON.stringify(latest,null,2)],{type:"application/json"}));
  a.download="omran-video-project.json";
  a.click();
  URL.revokeObjectURL(a.href);
});

const renderForm=qs("#renderer"),renderStatus=qs("#render-status"),preview=qs("#preview"),videoDownload=qs("#video-download");
renderForm.addEventListener("submit",async e=>{
  e.preventDefault();
  renderStatus.textContent="جاري تصدير الفيديو…";
  preview.hidden=true;
  videoDownload.hidden=true;
  try{
    const res=await fetch("/api/render",{method:"POST",body:new FormData(renderForm)});
    if(!res.ok) throw new Error(await res.text());
    const data=await res.json();
    preview.src=data.url;
    preview.hidden=false;
    videoDownload.href=data.url;
    videoDownload.download="omran-video.mp4";
    videoDownload.hidden=false;
    renderStatus.textContent="تم التصدير بنجاح";
  }catch(err){renderStatus.textContent=err.message}
});

const sceneList=qs("#scene-list"),sceneTemplate=qs("#scene-template"),batchForm=qs("#batch-form"),batchStatus=qs("#batch-status");
function addScene(seed={}){
  if(sceneList.children.length>=20){batchStatus.textContent="الحد الأقصى 20 مشهدًا";return}
  const node=sceneTemplate.content.firstElementChild.cloneNode(true);
  node.querySelector(".scene-seconds").value=seed.seconds||3;
  node.querySelector(".scene-caption").value=seed.caption||"";
  node.querySelector(".remove-scene").addEventListener("click",()=>{
    if(sceneList.children.length===1){batchStatus.textContent="لازم يكون فيه مشهد واحد على الأقل";return}
    node.remove(); updateScenes();
  });
  node.querySelector(".scene-seconds").addEventListener("input",updateScenes);
  sceneList.append(node);
  updateScenes();
}

function updateScenes(){
  [...sceneList.children].forEach((card,i)=>{
    card.querySelector(".scene-title").textContent=`المشهد ${i+1}`;
    card.querySelector(".remove-scene").disabled=sceneList.children.length===1;
  });
  const total=[...sceneList.querySelectorAll(".scene-seconds")].reduce((sum,input)=>sum+(Number(input.value)||0),0);
  qs("#scene-total").textContent=`${total} ثانية`;
}

qs("#add-scene").addEventListener("click",()=>addScene());
addScene();

async function loadTemplates(){
  try{
    const res=await fetch("/api/templates");
    if(!res.ok)return;
    const items=await res.json();
    const select=qs("#template-select");
    select.replaceChildren(...items.map(item=>{
      const option=document.createElement("option");
      option.value=item.ID;
      option.textContent=item.Name;
      return option;
    }));
  }catch{}
}

batchForm.addEventListener("submit",async e=>{
  e.preventDefault();
  const cards=[...sceneList.children];
  const scenes=cards.map(card=>({
    seconds:Number(card.querySelector(".scene-seconds").value),
    caption:card.querySelector(".scene-caption").value.trim()
  }));
  const data=new FormData();
  data.append("manifest",JSON.stringify({
    name:batchForm.elements.name.value.trim(),
    template:batchForm.elements.template.value,
    scenes
  }));
  cards.forEach((card,i)=>data.append(`scene_${i}`,card.querySelector(".scene-image").files[0]));
  if(batchForm.elements.audio.files[0])data.append("audio",batchForm.elements.audio.files[0]);
  if(batchForm.elements.watermark.files[0])data.append("watermark",batchForm.elements.watermark.files[0]);
  batchStatus.textContent="جاري إضافة المهمة…";
  try{
    const res=await fetch("/api/batch",{method:"POST",body:data});
    if(!res.ok)throw new Error(await res.text());
    const task=await res.json();
    batchStatus.textContent=`تمت إضافة المهمة: ${task.name}`;
    await loadJobs();
  }catch(err){batchStatus.textContent=err.message}
});

const statusNames={queued:"في الانتظار",running:"جاري التصدير",done:"اكتملت",failed:"فشلت"};
function jobCard(task){
  const card=document.createElement("article");
  card.className=`job-card job-${task.status}`;
  const top=document.createElement("div");top.className="job-top";
  const title=document.createElement("strong");title.textContent=task.name;
  const badge=document.createElement("span");badge.className="status-badge";badge.textContent=statusNames[task.status]||task.status;
  top.append(title,badge);
  const meta=document.createElement("small");meta.textContent=new Date(task.created_at).toLocaleString("ar-EG");
  card.append(top,meta);
  if(task.error){const error=document.createElement("p");error.className="error";error.textContent=task.error;card.append(error)}
  if(task.output_url){
    const actions=document.createElement("div");actions.className="job-actions";
    const video=document.createElement("a");video.href=task.output_url;video.download="";video.textContent="تحميل MP4";
    actions.append(video);
    if(task.srt_url){const srt=document.createElement("a");srt.href=task.srt_url;srt.download="";srt.textContent="تحميل SRT";actions.append(srt)}
    card.append(actions);
  }
  return card;
}

async function loadJobs(){
  try{
    const res=await fetch("/api/jobs",{cache:"no-store"});
    if(!res.ok)return;
    const items=await res.json();
    const list=qs("#job-list");
    if(!items.length){list.innerHTML='<p class="empty">لا توجد مهام حتى الآن.</p>';return}
    list.replaceChildren(...items.map(jobCard));
  }catch{}
}

qs("#refresh-jobs").addEventListener("click",loadJobs);
loadTemplates();
loadJobs();
setInterval(loadJobs,2500);
