'use strict';

var modifyInfo = function(info) {
    var modified = false;
    if ('audio_ads' in info) {
        info['audio_ads']['day_limit'] = 0;
        info['audio_ads']['types_allowed'] = [];
        info['audio_ads']['sections'] = [];
        modified = true;
    }
    if (!('profiler_enabled' in info) || info['profiler_enabled']) {
        info['profiler_enabled'] = false;
        modified = true;
    }
    if ('music_intro' in info && info['music_intro']) {
        info['music_intro'] = false;
        modified = true;
    }
    if ('settings' in info) {
        var settings = info['settings'];
        for (var i = 0, len = settings.length; i < len; i++) {
            if (['audio_ads', 'audio_restrictions'].includes(settings[i]['name'])) {
                settings[i]['available'] = false;
                modified = true;
            }
        }
    }
    return modified;
}

module.exports.transform = function(response, req, res) {
    if (req.url == '/method/execute.getNewsfeedSmart') {
        var json = response.json;
        if ('response' in json && 'items' in json['response']) {
            var items = json['response']['items'];
            var initialLength = items.length;
            for (var i = 0, len = items.length; i < len; i++) {
                if (items[i]['type'] == 'ads' || (items[i]['type'] == 'post' && items[i]['marked_as_ads'] == 1)) {
                    items.splice(i, 1);
                    len--;
                }
            }
            if (initialLength != items.length)
                response.json = json;
        }
    } else if (req.url == '/method/execute.getCountersAndInfo') {
        let json = response.json;
        if ('response' in json) {
            let modified = modifyInfo(json['response']);
            if (modified)
                response.json = json;
        }
    } else if (req.url == '/method/execute.getUserInfo') {
        let json = response.json;
        if ('response' in json && 'info' in json['response']) {
            let modified = modifyInfo(json['response']['info']);
            if (modified)
                response.json = json;
        }
    }
};
