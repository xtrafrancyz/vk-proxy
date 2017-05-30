'use strict';

var fs = require('fs');

var users = {
    fileName: 'users.json',
    container: new Set(),
    loaded: false
};
var realtime = {
    uniques: new Set(),
    requests: 0
};

fs.readFile(users.fileName, function read(err, data) {
    if (err) {
        users.loaded = true;
        return;
    }
    let arr = JSON.parse(data);
    for (var token of arr)
        users.container.add(token);
    users.loaded = true;
});

setInterval(function() {
    console.info('Requests: ' + realtime.requests + '. Users online: ' + realtime.uniques.size + '. Users total: ' + users.container.size);
    realtime.requests = 0;
    realtime.uniques.clear();
}, 60 * 1000);

module.exports.onRequest = function(req, res) {
    realtime.requests++;
};

module.exports.transform = function(response, req, res) {
    if (req.body && req.body['access_token'])
        realtime.uniques.add(req.body['access_token']);

    if (req.url == '/method/execute.getUserInfo') {
        console.log(response.raw);
        let json = response.json;
        if ('response' in json) {
            let lastSize = users.container.size;
            let profile = json['response']['profile'];
            users.container.add(profile['id']);
            if (users.loaded && lastSize != users.container.size) {
                console.log('New user - ' + profile['id'] + ' (' + profile['first_name'] + ' ' + profile['last_name'] + ')');
                fs.writeFile(users.fileName, JSON.stringify(Array.from(users.container)));
            }
        }
    }
};
