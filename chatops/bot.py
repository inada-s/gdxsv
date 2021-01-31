from discord.ext import commands
from socket import gethostname
import discord
import ops
import os
import sqlite3
import urllib.request

bot = commands.Bot(
    command_prefix='op ',
    activity=discord.Game(gethostname()),
)


@bot.command()
async def ping(ctx):
    await ctx.send('pong')


@bot.command()
async def update_masterdata(ctx):
    await ctx.send("Updating masterdata...")
    try:
        tables = ops.normalize_tables(ops.download_masterdata())
        conn = sqlite3.connect(os.getenv("GDXSV_DB_NAME"))
        with conn:
            ops.insert_sqlite(conn, tables)
            conn.commit()
    except Exception as e:
        await ctx.send("Failed to update masterdata")
        await ctx.send(str(e))

    req = urllib.request.Request("http://localhost:9880/ops/reload")
    with urllib.request.urlopen(req) as res:
        await ctx.send(res.read())


if __name__ == '__main__':
    assert os.getenv("GDXSV_DB_NAME")
    assert os.getenv("GDXSV_DISCORD_TOKEN")
    assert os.getenv("GDXSV_SERVICE_KEY")
    assert os.getenv("GDXSV_SPREADSHEET_ID")
    bot.run(os.getenv("GDXSV_DISCORD_TOKEN"))
