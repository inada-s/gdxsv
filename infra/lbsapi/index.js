axios = require("axios");

/**
 * Responds to any HTTP request.
 *
 * @param {!express:Request} req HTTP request context.
 * @param {!express:Response} res HTTP response context.
 */
exports.cloudFunctionEntryPoint = async (req, res) => {
    if (req.method !== "GET") {
        res.status(400).send('bad request');
        return;
    }
    const response = await axios.get('http://zdxsv.net:9880/lbs/' + req.url);
    res.header('Access-Control-Allow-Origin', "*");
    res.status(200).send(response.data);
}
