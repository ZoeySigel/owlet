// Local development only; isolated cluster, bound to loopback, no production data.
import EmbeddedPostgres from '../.tools/runtime/node_modules/embedded-postgres/dist/index.js';
import fs from 'node:fs';
import path from 'node:path';
const dir=path.resolve('.tools/test-pg');
const db=new EmbeddedPostgres({databaseDir:dir,user:'postgres',password:'owlet-local-tests-only',port:55432,persistent:true,authMethod:'scram-sha-256',postgresFlags:['-h','127.0.0.1','-c','shared_buffers=64MB','-c','max_connections=30'],onLog:()=>{},onError:s=>console.error(String(s).slice(0,200))});
if(!fs.existsSync(path.join(dir,'PG_VERSION')))await db.initialise();
await db.start();
console.log('Isolated PostgreSQL ready at 127.0.0.1:55432');
let closing=false;async function close(){if(closing)return;closing=true;await db.stop();process.exit(0)}
process.on('SIGINT',close);process.on('SIGTERM',close);
setInterval(()=>{},60000);
