'use strict';

module.exports.transform = function(strData, req, res) {
    if (req.url == '/method/execute.getNewsfeedSmart') {
        var json = JSON.parse(strData);
        var items = json['response']['items'];
        if (items) {
            var initialLength = items.length;
            for (var i = 0, len = items.length; i < len; i++) {
                if (items[i]['type'] == 'ads' || (items[i]['type'] == 'post' && items[i]['marked_as_ads'] == 1)) {
                    items.splice(i, 1);
                    len--;
                }
            }
            if (initialLength != items.length)
                strData = JSON.stringify(json);
        }
    } else if (req.url == '/method/execute.getCountersAndInfo') {
        var modified = false;
        var json = JSON.parse(strData);
        var response = json['response'];
        if (response['audio_ads']) {
            response['audio_ads']['day_limit'] = 0;
            response['audio_ads']['types_allowed'] = [];
            response['audio_ads']['sections'] = [];
        }
        if (response['profiler_enabled'])
            response['profiler_enabled'] = false;
        var settings = response['settings'];
        if (settings) {
            for (var i = 0, len = settings.length; i < len; i++) {
                if (['audio_ads', 'audio_restrictions'].includes(settings[i]['name'])) {
                    settings[i]['available'] = false;
                    modified = true;
                }
            }
        }
        if (modified)
            strData = JSON.stringify(json);
    }
    return strData;
};
