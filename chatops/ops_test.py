import unittest
import ops
import sqlite3


class OpsTestCase(unittest.TestCase):
    def test_normalize_tables(self):
        self.assertEqual(
            {'m_strings': [
                ['key', 'value'],
                ['a', 'a'],
                ['c', ''],
                ['x', 'x']]},
            ops.normalize_tables([[
                [],
                [],
                ['', '', '', '', '@m_strings'], ['', '', '', '', 'key', 'value'], ['', '', '', '', 'a', 'a'],
                ['', '', '', '', 'c'], ['', '', '', '', 'x', 'x'], ['', '', '', '', '']
            ]])
        )

    def test_insert_sqlite(self):
        conn = sqlite3.connect(':memory:')
        # conn.row_factory = dict_factory
        conn.execute("""
CREATE TABLE users (
    id INT,
    name TEXT NOT NULL,
    age INTEGER NOT NULL default 0,
    created_at DATETIME,
    PRIMARY KEY (id)
);
""")
        ops.insert_sqlite(conn, {
            'users': [
                ['id', 'name', 'age', 'created_at'],
                ['1', 'A', '20', '2000-01-01 00:00:00'],
                ['2', 'B', '19', '2001-01-01 00:00:01'],
                ['3', 'C', '', '2001-01-01 00:00:01'],
            ]
        })

        cur = conn.cursor()
        a = cur.execute("SELECT * FROM users").fetchall()
        self.assertEqual([
            (1, 'A', 20, '2000-01-01 00:00:00'),
            (2, 'B', 19, '2001-01-01 00:00:01'),
            (3, 'C', 0, '2001-01-01 00:00:01'),
        ], a)



def dict_factory(cursor, row):
    d = {}
    for idx, col in enumerate(cursor.description):
        d[col[0]] = row[idx]
    return d


if __name__ == '__main__':
    unittest.main()
