(()=>{
  "use strict";

  const clean=(value)=>String(value??"").trim();
  const make=(tag,className,text)=>{
    const node=document.createElement(tag);
    if(className)node.className=className;
    if(text!=null)node.textContent=text;
    return node;
  };
  const css=(node,rules)=>{Object.assign(node.style,rules);return node};

  const apiPrices=[
    {provider:"Anthropic",model:"Claude Sonnet 5",input:2,output:10},
    {provider:"OpenAI",model:"GPT-5.6 Sol",input:4,output:20},
    {provider:"Anthropic",model:"Claude Opus 5",input:5,output:25},
  ];
  const usd=(value)=>Number(value||0).toLocaleString("en-US",{style:"currency",currency:"USD",minimumFractionDigits:2,maximumFractionDigits:2});
  const tokens=(value)=>{
    const parsed=Number(clean(value).replaceAll(",",""));
    return Number.isFinite(parsed)&&parsed>0?parsed:0;
  };

  function enhanceAnalytics(){
    const kpis=document.getElementById("llm-kpis");
    if(!kpis)return;
    const spend=[...kpis.querySelectorAll(":scope > .llm-summary-item")].find((item)=>clean(item.querySelector(".k")?.textContent)==="Token spend");
    const raw=clean(spend?.querySelector(".v")?.textContent);
    const [promptRaw,completionRaw]=raw.split("/").map((item)=>item.trim());
    const prompt=tokens(promptRaw);
    const completion=tokens(completionRaw);
    let panel=document.getElementById("llm-api-equivalent");
    if(!prompt&&!completion){
      if(panel)panel.hidden=true;
      return;
    }
    if(!panel){
      panel=make("section","llm-api-equivalent");
      panel.id="llm-api-equivalent";
      css(panel,{borderBottom:"1px solid var(--line)",background:"var(--bay-2)"});
      kpis.insertAdjacentElement("afterend",panel);
    }
    panel.hidden=false;
    panel.replaceChildren();

    const head=css(make("div","llm-api-head"),{display:"flex",alignItems:"flex-start",justifyContent:"space-between",gap:"12px",padding:"8px 10px",borderBottom:"1px solid var(--line)"});
    const headCopy=make("div","");
    const title=css(make("strong","","If this ran on hosted APIs…"),{display:"block",color:"var(--text)",fontSize:"11px"});
    const note=css(make("span","","Equivalent cost for the token volume above using standard uncached text-token list pricing."),{display:"block",marginTop:"2px",color:"var(--muted)",fontSize:"9px"});
    const date=css(make("em","","Pricing Sep 2026"),{flex:"0 0 auto",padding:"2px 5px",border:"1px solid var(--line)",color:"var(--faint)",fontFamily:"var(--mono)",fontSize:"8px",fontStyle:"normal",textTransform:"uppercase"});
    headCopy.append(title,note);
    head.append(headCopy,date);
    panel.append(head);

    const grid=css(make("div","llm-api-grid"),{display:"grid",gridTemplateColumns:"repeat(auto-fit,minmax(170px,1fr))",gap:"1px",background:"var(--line)"});
    for(const price of apiPrices){
      const cost=(prompt/1_000_000)*price.input+(completion/1_000_000)*price.output;
      const cell=css(make("div","llm-api-cost"),{display:"grid",minWidth:"0",padding:"8px 10px",background:"var(--bay)"});
      const provider=css(make("span","",price.provider),{color:"var(--faint)",fontSize:"8px",fontWeight:"800",letterSpacing:".05em",textTransform:"uppercase"});
      const model=css(make("strong","",price.model),{marginTop:"1px",color:"var(--muted)",fontSize:"10px"});
      const costNode=css(make("b","",usd(cost)),{marginTop:"5px",color:"var(--text)",fontFamily:"var(--mono)",fontSize:"16px"});
      const rate=css(make("small","",`$${price.input}/M in · $${price.output}/M out`),{marginTop:"2px",color:"var(--faint)",fontFamily:"var(--mono)",fontSize:"8px"});
      cell.append(provider,model,costNode,rate);
      grid.append(cell);
    }
    panel.append(grid);
    const foot=css(make("div","llm-api-foot"),{padding:"6px 10px",color:"var(--muted)",fontSize:"9px"});
    const zero=css(make("b","","$0 hosted API spend"),{color:"var(--text)",fontFamily:"var(--mono)"});
    foot.append(document.createTextNode("Local inference: "),zero,document.createTextNode(" · hardware and electricity are not included in this comparison."));
    panel.append(foot);
  }

  const analyticsRoot=document.getElementById("llm-kpis");
  if(analyticsRoot){
    new MutationObserver(enhanceAnalytics).observe(analyticsRoot,{childList:true,subtree:true,characterData:true});
    enhanceAnalytics();
  }

  const root=document.getElementById("detail-play");
  if(!root)return;

  const directRows=(box)=>[...box.children].filter((node)=>node.classList&&node.classList.contains("prow"));
  const parts=(row)=>{
    const spans=row.querySelectorAll(":scope > span");
    return {label:clean(spans[0]&&spans[0].textContent),value:clean(spans[1]&&spans[1].textContent)};
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
