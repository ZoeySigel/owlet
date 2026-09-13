import assert from 'node:assert/strict';
const base=process.env.TEST_BASE_URL||'http://127.0.0.1:18080';
const origin=process.env.TEST_WEB_ORIGIN||'http://localhost:5173';
async function call(path,{method='GET',body,cookie='',key}={}){const res=await fetch(base+'/api'+path,{method,headers:{Origin:origin,...(body?{'Content-Type':'application/json'}:{}),...(cookie?{Cookie:cookie}:{}),...(key?{'Idempotency-Key':key}:{})},body:body?JSON.stringify(body):undefined});const value=await res.json();return {status:res.status,value,cookie:res.headers.get('set-cookie')?.split(';')[0]}}
const admin=await call('/auth/login',{method:'POST',body:{username:process.env.TEST_ADMIN_USER||'demo-admin',password:process.env.TEST_ADMIN_PASSWORD||'owlet-local-demo-only'}});assert.equal(admin.status,200);
const suffix=Date.now();async function register(name){const i=await call('/admin/invite',{method:'POST',body:{},cookie:admin.cookie});assert.equal(i.status,200);const u=await call('/auth/register',{method:'POST',body:{username:name+suffix,password:'owlet-test-password',invite:i.value.invite}});assert.equal(u.status,200);return u}
const one=await register('one'),two=await register('two');
const body={name:'API test',selling:'真实卖点',pages:[],assetId:'',color:'#ff2442'};
const d=await call('/documents',{method:'POST',cookie:one.cookie,body:{kind:'project',body}});assert.equal(d.status,200);
assert.equal((await call('/documents/'+d.value.id,{method:'PUT',cookie:two.cookie,body:{kind:'project',body,revision:1}})).status,409);
assert.equal((await call('/documents/'+d.value.id,{method:'DELETE',cookie:two.cookie})).status,404);
assert.equal((await call('/documents',{cookie:two.cookie})).value.length,0);
const key=crypto.randomUUID(),input={project_id:d.value.id,kind:'text',prompt:'test',page:-1};
const first=await call('/jobs',{method:'POST',cookie:one.cookie,body:input,key});assert.equal(first.status,201);
const duplicate=await call('/jobs',{method:'POST',cookie:one.cookie,body:input,key});assert.equal(duplicate.value.id,first.value.id);
assert.equal((await call('/jobs',{method:'POST',cookie:two.cookie,body:input,key:crypto.randomUUID()})).status,404);
const stream=await fetch(base+'/api/jobs/'+first.value.id+'/events',{headers:{Cookie:two.cookie}});assert.equal(stream.status,404);
let job;for(let i=0;i<30;i++){job=(await call('/jobs',{cookie:one.cookie})).value.find(j=>j.id===first.value.id);if(job.state==='succeeded')break;await new Promise(r=>setTimeout(r,500))}assert.equal(job.state,'succeeded');assert.equal(job.charged,10000);
const usage=(await call('/usage',{cookie:one.cookie})).value;assert.equal(usage.daily.reserved,0);assert.equal(usage.daily.spent,10000);
const versions=(await call('/documents/'+d.value.id+'/versions',{cookie:one.cookie})).value;assert.equal(versions.length,1);
assert.equal((await call('/admin',{cookie:one.cookie})).status,403);
await call('/admin/user',{method:'POST',cookie:admin.cookie,body:{id:two.value.id,disabled:true}});assert.equal((await call('/me',{cookie:two.cookie})).status,401);
await call('/auth/logout',{method:'POST',cookie:one.cookie,body:{}});assert.equal((await call('/me',{cookie:one.cookie})).status,401);
console.log('PASS: registration, owner isolation, job idempotency, SSE ownership, mock completion, quota settlement, versions, admin authorization, disable, logout');
