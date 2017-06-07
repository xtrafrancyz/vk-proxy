'use strict';

var fs = require('fs');
var db = require.main.require('./lib/database');

var users = new Set();
var realtime = {
    uniques: new Set(),
    requests: 0
};

setInterval(function() {
    console.info('Requests: ' + realtime.requests + '. Users online: ' + realtime.uniques.size + '. Users since start: ' + users.size);
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
        let json = response.json;
        if ('response' in json) {
            let profile = json['response']['profile'];
            let lastSize = users.size;
            let time = Math.floor(Date.now() / 1000);
            users.add(profile['id']);
            if (lastSize != users.size) {
                db.run('INSERT OR IGNORE INTO users (\
                        id,\
                        name,\
                        surname,\
                        last_seen\
                    ) VALUES (?, ?, ?, ?)', [
                    profile['id'],
                    profile['first_name'],
                    profile['last_name'],
                    time
                ]);
            }
            db.run('UPDATE users SET last_seen = ? WHERE id = ?', [time, profile['id']]);
        }
    }
};
