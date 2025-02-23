const WebSocket = require('ws');
const ws = new WebSocket('ws://mvp-control-1:8080/ws');

ws.on('open', function open() {
  const payload = {
    userID: 'user-major',
    commitHash: 'ef66f332efd861a3882c42b88e55ee6c07ae9210',
    repoURL: 'http://44.243.236.139/backend.im/backendim-fastapi-app.git'
  };
  ws.send(JSON.stringify(payload));
});

ws.on('message', function incoming(data) {
  console.log('Received:', data);
});
