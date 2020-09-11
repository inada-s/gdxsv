'use strict';

const {Logging} = require('@google-cloud/logging');
const logging = new Logging();
const log = logging.log(process.env.FUNCTION_NAME);
const logmetadata = {
    resource: {
        type: 'cloud_function',
        labels: {
            function_name: process.env.FUNCTION_NAME,
            project: process.env.GCP_PROJECT,
            region: process.env.FUNCTION_REGION
        },
    },
};

function debug_entry(text) {
    return log.entry({logmetadata, severity: "DEBUG"}, text);
}

function info_entry(text) {
    return log.entry({logmetadata, severity: "INFO"}, text);
}

function notice_entry(text) {
    return log.entry({logmetadata, severity: "NOTICE"}, text);
}

function warning_entry(text) {
    return log.entry({logmetadata, severity: "WARNING"}, text);
}

function error_entry(text) {
    return log.entry({logmetadata, severity: "ERROR"}, text);
}

exports.cloudFunctionEntryPoint = async (req, res) => {
    if (req.method === "GET") {
        res.status(400).send('bad request');
        return;
    }
    const entries = [];
    const logdata = req.body.toString();
    const lines = logdata.split("\n");
    for (let line of lines) {
        line = line.trim();
        if (!line) {
            continue;
        }
        if (line.includes(" I[")) {
            entries.push(info_entry(line));
        } else if (line.includes(" N[")) {
            entries.push(notice_entry(line));
        } else if (line.includes(" W[")) {
            entries.push(warning_entry(line));
        } else if (line.includes(" E[")) {
            entries.push(error_entry(line));
        } else {
            entries.push(debug_entry(line));
        }
    }
    res.status(204).send();
    await log.write(entries);
}