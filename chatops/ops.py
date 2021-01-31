import os
import sqlite3
import sys
from typing import *

from google.oauth2 import service_account
from googleapiclient.discovery import build


def get_spreadsheet_service():
    key = os.getenv("GDXSV_SERVICE_KEY")
    assert key
    credentials = service_account.Credentials.from_service_account_file(key)
    scoped_credentials = credentials.with_scopes([
        'https://www.googleapis.com/auth/spreadsheets'
    ])
    service = build('sheets', 'v4', credentials=scoped_credentials)
    return service


def download_masterdata() -> List[List[List[str]]]:
    service = get_spreadsheet_service()
    spreadsheet_id = os.getenv('GDXSV_SPREADSHEET_ID')
    spreadsheet = service.spreadsheets().get(
        spreadsheetId=spreadsheet_id,
    ).execute()

    ranges = [
        f"{sheet['properties']['title']}!A1:Z1000" for sheet in spreadsheet['sheets']
    ]

    response = service.spreadsheets().values().batchGet(
        spreadsheetId=spreadsheet_id,
        ranges=ranges
    ).execute()

    return [vr["values"] for vr in response["valueRanges"]]


def normalize_tables(tables: List[List[List[str]]]) -> Dict[str, List[List[str]]]:
    ret = dict()
    for rows in tables:
        reading = False
        current_table = []
        left, right = 0, 0

        for i in range(len(rows)):
            if not reading:
                left = -1
                for j in range(len(rows[i])):
                    if rows[i][j].startswith("@m_"):
                        left = j
                if left < 0:
                    continue

                current_table_name = rows[i][left][1:].strip()
                if len(rows) <= i + 1:
                    continue
                right = len(rows[i + 1])
                for j in range(len(rows[i + 1])):
                    if left <= j and not rows[i + 1][j]:
                        right = j

                current_table = []
                ret[current_table_name] = current_table
                reading = True
                continue

            r = rows[i][left:right]
            if any(r):
                while current_table and len(r) < len(current_table[0]):
                    r.append('')
                current_table.append(r)
            else:
                reading = False

    return ret


def insert_sqlite(conn: sqlite3.Connection, tables: Dict[str, List[List[str]]]):
    for table, rows in tables.items():
        ints = {
            row[1] for row in conn.execute(f"pragma table_info('{table}')").fetchall() if row[2].upper() in "INTEGER"}
        columns = rows[0]
        for i in range(len(columns)):
            if columns[i] in ints:
                for row in rows[1:]:
                    row[i] = int(row[i]) if row[i] else 0
        conn.execute(f"DELETE FROM {table}")
        conn.executemany(f"INSERT INTO {table} VALUES ({','.join(['?'] * len(columns))})", rows[1:])


if __name__ == '__main__':
    if len(sys.argv) == 1:
        tables = download_masterdata()
        normalized_tables = normalize_tables(tables)
        for table, values in normalized_tables.items():
            print(table, values)
    else:
        if sys.argv[1] == "insert_sqlite":
            tables = download_masterdata()
            normalized_tables = normalize_tables(tables)
            conn = sqlite3.connect(sys.argv[2])
            with conn:
                insert_sqlite(conn, normalized_tables)
                conn.commit()
