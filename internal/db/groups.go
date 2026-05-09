package db

import (
	"database/sql"
	"fmt"
)

type Group struct {
	Path        string
	Name        string
	DefaultPath string
	DefaultTool string
	Expanded    bool
	SortOrder   int
}

func CreateGroup(conn *sql.DB, g Group) error {
	expanded := 0
	if g.Expanded {
		expanded = 1
	}
	_, err := conn.Exec(
		`INSERT INTO groups (path, name, default_path, default_tool, expanded, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		g.Path, g.Name, g.DefaultPath, g.DefaultTool, expanded, g.SortOrder,
	)
	return err
}

func GetGroup(conn *sql.DB, path string) (Group, error) {
	var g Group
	var expanded int
	err := conn.QueryRow(
		`SELECT path, name, default_path, default_tool, expanded, sort_order
		 FROM groups WHERE path = ?`, path,
	).Scan(&g.Path, &g.Name, &g.DefaultPath, &g.DefaultTool, &expanded, &g.SortOrder)
	if err != nil {
		return Group{}, fmt.Errorf("get group %q: %w", path, err)
	}
	g.Expanded = expanded == 1
	return g, nil
}

func ListGroups(conn *sql.DB) ([]Group, error) {
	rows, err := conn.Query(
		`SELECT path, name, default_path, default_tool, expanded, sort_order
		 FROM groups ORDER BY sort_order, path`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroups(rows)
}

// ChildGroups returns direct children of parentPath (one level deep only).
func ChildGroups(conn *sql.DB, parentPath string) ([]Group, error) {
	prefix := parentPath + "/%"
	deeperPrefix := parentPath + "/%/%"
	rows, err := conn.Query(
		`SELECT path, name, default_path, default_tool, expanded, sort_order
		 FROM groups WHERE path LIKE ? AND path NOT LIKE ?
		 ORDER BY sort_order, path`,
		prefix, deeperPrefix,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroups(rows)
}

func SetGroupExpanded(conn *sql.DB, path string, expanded bool) error {
	v := 0
	if expanded {
		v = 1
	}
	_, err := conn.Exec(`UPDATE groups SET expanded = ? WHERE path = ?`, v, path)
	return err
}

func RenameGroup(conn *sql.DB, path, newName string) error {
	_, err := conn.Exec(`UPDATE groups SET name = ? WHERE path = ?`, newName, path)
	return err
}

func DeleteGroup(conn *sql.DB, path string) error {
	_, err := conn.Exec(`DELETE FROM groups WHERE path = ?`, path)
	return err
}

func scanGroups(rows *sql.Rows) ([]Group, error) {
	var groups []Group
	for rows.Next() {
		var g Group
		var expanded int
		if err := rows.Scan(&g.Path, &g.Name, &g.DefaultPath, &g.DefaultTool, &expanded, &g.SortOrder); err != nil {
			return nil, err
		}
		g.Expanded = expanded == 1
		groups = append(groups, g)
	}
	return groups, rows.Err()
}
