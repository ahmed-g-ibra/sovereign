const h={blocked:0,unknown:1,conditional:2,allowed:3},O="Informational only. This is not legal advice, and it is not a substitute for advice from a qualified practitioner in the relevant jurisdiction. Every claim links to the entry it came from; check the primary source before relying on it.";function x(t,s,o){const e=t.countries??[];if(!e.includes("*")&&!e.includes(s))return!1;const a=t.data_classes??[];return a.includes("*")?!0:o.some(v=>a.includes(v))}const q=t=>t==="localised"?"conditional":t??"allowed",S=t=>Object.keys(h).find(s=>h[s]===t)??"allowed";function A(t){const s=new Set;return t.filter(o=>s.has(o.key)?!1:s.add(o.key))}function C(t){const s=Object.values(t);return s.some(o=>o.status==="draft")?"draft":s.some(o=>o.stale)?"stale":s.every(o=>o.status==="verified")?"verified":"reviewed"}function M({userCountries:t,dataClasses:s,entries:o,rules:e,candidateRegions:a,latency:v,window:R,probeCountries:D,now:L}){t=t.map(i=>i.toUpperCase());const m=[],w=[],_=[];let d=null;for(const i of t){const n=o[i];if(!n){m.push(`No residency entry for ${i}; it was ignored rather than guessed at.`);continue}const c=(e.rules??[]).filter(l=>x(l,i,s));_.push(...c);const u=[];for(const l of c)for(const p of l.obligations??[]){const y=e.obligations?.[p]??{};u.push({key:p,label:y.label??p,detail:y.detail??""})}for(const l of c)if(l.verdict==="localised"&&l.requires_region_in){const p=new Set(l.requires_region_in);d=d===null?p:new Set([...d].filter(y=>p.has(y)))}const g=c.length?Math.min(...c.map(l=>h[q(l.verdict)]??3)):3;w.push({country:i,country_name:n.country_name,kb_status:n.status,regulator:n.regulator,instrument:n.instrument,verdict:S(g),obligations:A(u),notes:c.filter(l=>l.note).map(l=>l.note.trim()),open_questions:n.open_questions??[],citation:`knowledge/residency/${i}.md`,primary_source:n.primary_source}),n.status==="draft"&&m.push(`${n.country_name} is a DRAFT entry written from secondary sources and not checked against the instrument.`),n.stale&&m.push(`${n.country_name} was last verified ${n.days_since_verified} days ago.`)}d!==null&&d.size===0&&m.push("Two localisation rules conflict: no region can satisfy both. This workload may not be lawfully deployable as specified.");const N=_.length?Math.min(..._.map(i=>h[q(i.verdict)]??3)):3,b=a.map(i=>{const n=[];let c=S(N);if(d!==null){const u=[...d].sort().join("/");d.has(i.country)?n.push(`Satisfies the localisation requirement (${u}).`):(c="blocked",n.push(`Localisation requires a region in ${u}; this region is in ${i.country}.`))}return c==="conditional"&&n.length===0&&n.push("Permitted if the listed obligations are met."),c==="unknown"&&n.push("At least one jurisdiction's position could not be established. Treated as unknown rather than assumed permissive."),c==="allowed"&&n.length===0&&n.push("No obligation found in the rules for this combination."),{target_id:i.id,provider:i.provider,region:i.region,country:i.country,city:i.city??null,verdict:c,reasons:n,latency:v[i.id]??null}});b.sort((i,n)=>{const c=(h[i.verdict]??9)*-1,u=(h[n.verdict]??9)*-1;if(c!==u)return c-u;const g=i.latency?.p95_ms??1/0,l=n.latency?.p95_ms??1/0;return g!==l?g-l:i.target_id<n.target_id?-1:i.target_id>n.target_id?1:0});const k=b.filter(i=>i.latency).length;return k===0&&m.push("No latency measurements in this window, so the ranking is by verdict only."),{generated_at:L??new Date().toISOString(),status:C(o),warnings:m,inputs:{user_countries:t,data_classes:s},jurisdictions:w,regions:b,evidence:{window:R,probe_countries:D,regions_with_measurements:k,note:"Percentiles over successful probes only, quarantined probes excluded."},rules_version:e.version,disclaimer:O}}const f=JSON.parse(document.getElementById("advisor-data").textContent),E=document.getElementById("advisor-form"),$=document.getElementById("advisor-result"),I=t=>[...E.querySelectorAll(`input[name="${t}"]:checked`)].map(s=>s.value),r=t=>String(t??"").replace(/[&<>"]/g,s=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"})[s]),T=t=>t==null?"—":t.toFixed(1);function V(t){const s=f.matrix.filter(a=>t.includes(a.probe_country)),o=s.length?s:f.matrix,e={};for(const a of o)e[a.target_id]||(e[a.target_id]={samples:a.samples,loss_pct:a.loss_pct,p50_ms:a.p50_ms,p95_ms:a.p95_ms});return{latency:e,probeCountries:s.length?t:[]}}function P(t){const s=t.regions.map(e=>`
        <tr>
          <td>${r(e.region)}<span class="muted small"> ${r(e.city??"")}</span></td>
          <td>${r(e.provider)}</td>
          <td>${r(e.country)}</td>
          <td><span class="pill ${e.verdict}">${e.verdict}</span></td>
          <td class="num">${T(e.latency?.p50_ms)}</td>
          <td class="num">${T(e.latency?.p95_ms)}</td>
          <td class="small muted">${r(e.reasons[0]??"")}</td>
        </tr>`).join(""),o=t.jurisdictions.map(e=>`
        <div class="card">
          <h3 style="margin-top:0">${r(e.country_name)}
            <span class="pill ${e.verdict}">${e.verdict}</span>
            <span class="pill ${e.kb_status}">${e.kb_status}</span>
          </h3>
          <p class="small"><strong>${r(e.instrument)}</strong><br>
             <span class="muted">${r(e.regulator)}</span></p>
          ${e.obligations.length?`<p class="small"><strong>Obligations</strong></p>
            <ul class="small">${e.obligations.map(a=>`<li><strong>${r(a.label)}</strong> — ${r(a.detail)}</li>`).join("")}</ul>`:""}
          ${e.open_questions.length?`<details><summary class="muted small">
              ${e.open_questions.length} open question(s)</summary>
              <ul class="small">${e.open_questions.map(a=>`<li>${r(a)}</li>`).join("")}</ul></details>`:""}
          <p class="small muted">Source: <code>${r(e.citation)}</code></p>
        </div>`).join("");$.innerHTML=`
        <h2>Candidate regions</h2>
        ${t.warnings.length?`<div class="banner"><ul class="small" style="margin:0">
          ${t.warnings.map(e=>`<li>${r(e)}</li>`).join("")}</ul></div>`:""}
        <table>
          <thead><tr><th>Region</th><th>Provider</th><th>Country</th><th>Verdict</th>
            <th class="num">p50</th><th class="num">p95</th><th>Why</th></tr></thead>
          <tbody>${s}</tbody>
        </table>
        <p class="muted small">
          Latency measured over ${r(t.evidence.window)} from
          ${t.evidence.probe_countries.length?r(t.evidence.probe_countries.join(", ")):"every available vantage point"}.
          ${t.evidence.regions_with_measurements} of ${t.regions.length}
          regions have measurements.
        </p>
        <h2>Jurisdictions</h2>
        ${o||'<p class="muted">No applicable jurisdiction entries.</p>'}
        <p class="small muted">${r(t.disclaimer)}</p>`,$.scrollIntoView({behavior:"smooth",block:"start"})}E.addEventListener("submit",t=>{t.preventDefault();const s=I("country"),o=I("dataclass");if(!s.length||!o.length){$.innerHTML='<p class="muted">Pick at least one country and one data class.</p>';return}const{latency:e,probeCountries:a}=V(s);P(M({userCountries:s,dataClasses:o,entries:f.entries,rules:f.rules,candidateRegions:f.candidates,latency:e,window:f.window,probeCountries:a}))});
