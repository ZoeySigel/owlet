// Portable off-host backup encryption. Keep the key separately from encrypted backups.
import fs from 'node:fs';
import {randomBytes,createCipheriv,createDecipheriv} from 'node:crypto';
const [mode,input,output,keyPath]=process.argv.slice(2);
if(!['encrypt','decrypt'].includes(mode)||!keyPath)throw Error('Usage: node scripts/backup-crypto.mjs encrypt|decrypt input output key-file');
if(fs.existsSync(output))throw Error('Output already exists; refusing to overwrite');
if(!fs.existsSync(keyPath)){if(mode!=='encrypt')throw Error('Missing decryption key');fs.writeFileSync(keyPath,randomBytes(32).toString('hex'),{mode:0o600,flag:'wx'})}
const key=Buffer.from(fs.readFileSync(keyPath,'utf8').trim(),'hex');if(key.length!==32)throw Error('Invalid key');
const bytes=fs.readFileSync(input);
if(mode==='encrypt'){
 const iv=randomBytes(12), cipher=createCipheriv('aes-256-gcm',key,iv);
 const data=Buffer.concat([cipher.update(bytes),cipher.final()]);
 fs.writeFileSync(output,Buffer.concat([Buffer.from('OWLET001'),iv,cipher.getAuthTag(),data]),{mode:0o600,flag:'wx'});
}else{
 if(bytes.subarray(0,8).toString()!=='OWLET001')throw Error('Invalid backup header');
 const cipher=createDecipheriv('aes-256-gcm',key,bytes.subarray(8,20));cipher.setAuthTag(bytes.subarray(20,36));
 fs.writeFileSync(output,Buffer.concat([cipher.update(bytes.subarray(36)),cipher.final()]),{mode:0o600,flag:'wx'});
}
console.log('Backup '+mode+' successful');
