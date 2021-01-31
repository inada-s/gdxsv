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
async def ping(ctx: commands.Context):
    await ctx.send('pong')


@bot.command()
@commands.has_any_role("Moderator")
async def master(ctx: commands.Context):
    await ctx.send("https://docs.google.com/spreadsheets/d/" + os.getenv("GDXSV_SPREADSHEET_ID"))


@bot.command()
@commands.has_any_role("Moderator")
async def master_up(ctx: commands.Context):
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

    await ctx.send("Reloading masterdata...")

    req = urllib.request.Request("http://localhost:9880/ops/reload")
    with urllib.request.urlopen(req) as res:
        await ctx.send("Reload: " + res.read().decode('utf-8'))
    await ctx.send("Done")


if __name__ == '__main__':
    assert os.getenv("GDXSV_DB_NAME")
    assert os.getenv("GDXSV_DISCORD_TOKEN")
    assert os.getenv("GDXSV_SERVICE_KEY")
    assert os.getenv("GDXSV_SPREADSHEET_ID")
    bot.run(os.getenv("GDXSV_DISCORD_TOKEN"))
