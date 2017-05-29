'use strict';

var http = require('http'),
    connect = require('connect'),
    transformerProxy = require('transformer-proxy'),
    httpProxy = require('http-proxy'),
    querystring = require('querystring');

var config = require('./config');

require("console-stamp")(console, {
    pattern: 'dd.mm.yyyy HH:MM:ss',
    label: false,
    colors: {
        stamp: "yellow",
        metadata: "green"
    }
});

const handlers = [
    require('./lib/handler/url-replacer'),
    require('./lib/handler/ads-remover')
];
if (config.analytics)
    handlers.push(require('./lib/handler/analytics'));

var app = connect();
var proxy = httpProxy.createProxyServer({});

// Изменение запроса перед его выполнением у вк
/*proxy.on('proxyReq', function(proxyReq, req, res, options) {
    proxyReq.removeHeader('Accept-Encoding');
});*/

// Изменение заголовков ответа
proxy.on('proxyRes', function(proxyRes, req, res) {
    delete proxyRes.headers['set-cookie'];
});

// Изменение тела ответа от вк, замена ссылочек
app.use(transformerProxy(function(data, req, res) {
    let strData = data.toString('utf8');

    for (var i = 0, len = handlers.length; i < len; i++)
        strData = handlers[i].transform(strData, req, res);

    return Buffer.from(strData, 'utf8');
}));

app.use(function(req, res) {
    let target;
    if (req.url.substr(0, 7) == '/vk.com') {
        target = 'https://vk.com' + req.url.substr(7);
    } else {
        target = 'https://api.vk.com' + req.url;
    }

    if (req.method == 'POST') {
        var body = [];
        req.on('data', function(chunk) {
            body.push(chunk);
        }).on('end', function() {
            req.body = querystring.parse(Buffer.concat(body).toString());
        });
    }

    for (var i = 0, len = handlers.length; i < len; i++)
        if (handlers[i].onRequest)
            handlers[i].onRequest(req, res);

    delete req.headers['accept-encoding'];

    console.log(target);
    proxy.web(req, res, {
        target: target,
        ignorePath: true,
        secure: false,
        xfwd: true,
        toProxy: false,
        changeOrigin: true,
        hostRewrite: true,
        autoRewrite: false,
        proxyTimeout: 60000
    });
});

proxy.on('error', function(err, req, res) {
    res.writeHead(500, {
        'Content-Type': 'text/plain'
    });
    res.end('Something went wrong. Proxy server does not work');
    console.log('Error: ', err.message);
});

http.createServer(app).listen(config.port, '127.0.0.1');
console.log("Listening on port " + config.port);
