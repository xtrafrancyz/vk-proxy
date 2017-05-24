'use strict';

var fs = require('fs');

var usersFile = 'users.json';

var users = {
    container: new Set(),
    loaded: false
};

fs.readFile(usersFile, function read(err, data) {
    if (err) {
        console.log(err.message);
        users.loaded = true;
        return;
    }
    let arr = JSON.parse(data);
    for (var token of arr)
        users.container.add(token);
    users.loaded = true;
});

module.exports.transform = function(strData, req, res) {
    if (req.url == '/method/execute.getUserInfo') {
        try {
            let json = JSON.parse(strData);
            let lastSize = users.container.size;
            users.container.add(json['response']['profile']['id']);
            if (users.loaded && lastSize != users.container.size) {
                console.log('New user - ' + json['response']['profile']['id']);
                fs.writeFile(usersFile, JSON.stringify(Array.from(users.container)));
            }
        } catch (e) {
            console.log(e.message);
        }
    }
    return strData;
};
