(()=>{
  "use strict";

  const root=document.getElementById("detail-play");
  if(!root)return;

  const clean=(value)=>String(value??"").trim();
  const directRows=(box)=>[...box.children].filter((node)=>node.classList&&node.classList.contains("prow"));
  const parts=(row)=>{
    const spans=row.querySelectorAll(":scope > span");
    return {label:clean(spans[0]&&spans[0].textContent),value:clean(spans[1]&&spans[1].textContent)};
  };
  const make=(tag,className,text)=>{
    const node=document.createElement(tag);
    if(className)node.className=className;
    if(text!=null)node.textContent=text;
    return node;
  };
  const metricRow=(label,value,warn=false)=>{
    const row=make("div","llm-metric-row");
    row.append(make("span","llm-metric-key",label),make("span",warn?"llm-metric-value llm-metric-warn":"llm-metric-value",value));
    return row;
  };
  const group=(title,rows,className="")=>{
    if(!rows.length)return null;
    const box=make("section",`llm-metric-group ${className}`.trim());
    box.append(make("h4","llm-metric-title",title));
    for(const row of rows)box.append(row);
    return box;
  };
  const numberPair=(value)=>{
    const match=clean(value).match(/^([\d,.]+)\s*\/\s*([\d,.]+)$/);
    return match?`${match[1]} prompt · ${match[2]} completion`:clean(value);
  };
  const latencyBreakdown=(value)=>{
    const useful=clean(value).split("/").map((item)=>item.trim()).filter((item)=>item&&!item.startsWith("—"));
    return useful.length?useful.join(" · "):"—";
  };
  const normalize=(label,value)=>{
    switch(label){
      case "call latency": return ["call",clean(value).replace(" / "," · ").replace(" all avg"," avg")];
      case "latency avg": return ["breakdown",latencyBreakdown(value)];
      case "last call tokens": return ["last tokens",clean(value).replace(" / "," · ")];
      case "tokens": return ["total tokens",numberPair(value)];
      case "repeat picks": return ["repeats",value];
      default: return [label,value];
    }
  };
  const nonzero=(value)=>{
    const match=clean(value).match(/-?[\d.]+/);
    return match&&Number(match[0])>0;
  };

  function enhance(){
    const nums=root.querySelector(":scope .pnums");
    if(!nums||nums.dataset.llmGrouped==="true")return;
    const rows=directRows(nums).map((row)=>parts(row)).filter((row)=>row.label);
    if(!rows.length)return;
    const byLabel=new Map(rows.map((row)=>[row.label,row.value]));
    if(!byLabel.has("model")&&!byLabel.has("call latency"))return;

    const shell=make("div","llm-play-metrics");
    shell.dataset.llmGrouped="true";

    const head=make("div","llm-play-head");
    const round=byLabel.get("round");
    const modelRaw=byLabel.get("model")||"—";
    const modelBits=modelRaw.split(" · ").map((item)=>item.trim()).filter(Boolean);
    if(round)head.append(make("strong","llm-play-round",`Round ${round}`));
    head.append(make("span","llm-play-model",modelBits[0]||"—"));
    if(modelBits[1])head.append(make("span","llm-play-route",modelBits.slice(1).join(" · ")));
    shell.append(head);

    const served=byLabel.get("served by");
    if(served){
      const split=served.lastIndexOf(" @ ");
      const servedModel=split>=0?served.slice(0,split):"";
      const endpoint=split>=0?served.slice(split+3):served;
      const endpointRow=make("div","llm-endpoint-row");
      endpointRow.append(make("span","llm-endpoint-label",servedModel&&servedModel!==modelBits[0]?`served ${servedModel}`:"endpoint"),make("code","llm-endpoint-value",endpoint));
      shell.append(endpointRow);
    }

    const timingLabels=["call latency","latency avg","prefill","decode","server overhead"];
    const usageLabels=["last call tokens","tokens","offered","repeat picks"];
    const reliabilityLabels=["rejected","transport","fallbacks"];
    const consumed=new Set(["round","model","served by",...timingLabels,...usageLabels,...reliabilityLabels]);
    const timing=[];
    for(const label of timingLabels){
      if(!byLabel.has(label))continue;
      const [key,value]=normalize(label,byLabel.get(label));
      if(key==="breakdown"&&value==="—")continue;
      timing.push(metricRow(key,value));
    }
    const usage=[];
    for(const label of usageLabels){
      if(!byLabel.has(label))continue;
      const [key,value]=normalize(label,byLabel.get(label));
      usage.push(metricRow(key,value));
    }
    const columns=make("div","llm-metric-columns");
    const timingBox=group("Timing",timing);
    const usageBox=group("Usage",usage);
    if(timingBox)columns.append(timingBox);
    if(usageBox)columns.append(usageBox);
    if(columns.childElementCount)shell.append(columns);

    const reliability=[];
    for(const label of reliabilityLabels){
      if(!byLabel.has(label))continue;
      const value=byLabel.get(label);
      reliability.push(metricRow(label,value,nonzero(value)));
    }
    const reliabilityBox=group("Reliability",reliability,"llm-reliability");
    if(reliabilityBox)shell.append(reliabilityBox);

    const other=[];
    for(const {label,value} of rows){
      if(consumed.has(label))continue;
      other.push(metricRow(label,value));
    }
    const otherBox=group("Other",other);
    if(otherBox)shell.append(otherBox);

    nums.replaceWith(shell);
  }

  let queued=false;
  const schedule=()=>{
    if(queued)return;
    queued=true;
    requestAnimationFrame(()=>{queued=false;enhance()});
  };
  new MutationObserver(schedule).observe(root,{childList:true,subtree:true});
  schedule();
})();
