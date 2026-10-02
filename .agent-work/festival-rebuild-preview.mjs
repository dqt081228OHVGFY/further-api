import http from 'node:http';
http.createServer((req,res)=>{
 const old=new URL(req.url,'http://127.0.0.1:18317');
 const target=new URL('http://127.0.0.1:3001');
 target.pathname=old.pathname==='/'?'/console/':old.pathname;
 target.search=old.search;
 res.writeHead(307,{'Location':target.href,'Cache-Control':'no-store'});
 res.end('Full RelayFor preview: '+target.href);
}).listen(18317,'127.0.0.1',()=>console.log('Full preview redirects to 3001'));
