package db

import (
	"database/sql"
	"fmt"
	"strings"
)

type Group struct {
	Path               string
	Name               string
	DefaultPath        string
	DefaultTool        string
	ConductorSessionID string
	Expanded           bool
	SortOrder          int
}

func CreateGroup(conn *sql.DB, g Group) error {
	expanded := 0
	if g.Expanded {
		expanded = 1
	}
	_, err := conn.Exec(
		`INSERT INTO groups (path, name, default_path, default_tool, conductor_session_id, expanded, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		g.Path, g.Name, g.DefaultPath, g.DefaultTool, g.ConductorSessionID, expanded, g.SortOrder,
	)
	return err
}

func GetGroup(conn *sql.DB, path string) (Group, error) {
	var g Group
	var expanded int
	err := conn.QueryRow(
		`SELECT path, name, default_path, default_tool, conductor_session_id, expanded, sort_order
		 FROM groups WHERE path = ?`, path,
	).Scan(&g.Path, &g.Name, &g.DefaultPath, &g.DefaultTool, &g.ConductorSessionID, &expanded, &g.SortOrder)
	if err != nil {
		return Group{}, fmt.Errorf("get group %q: %w", path, err)
	}
	g.Expanded = expanded == 1
	return g, nil
}

func ListGroups(conn *sql.DB) ([]Group, error) {
	rows, err := conn.Query(
		`SELECT path, name, default_path, default_tool, conductor_session_id, expanded, sort_order
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
	escaped := strings.NewReplacer("%", `\%`, "_", `\_`).Replace(parentPath)
	prefix := escaped + "/%"
	deeperPrefix := escaped + "/%/%"
	rows, err := conn.Query(
		`SELECT path, name, default_path, default_tool, conductor_session_id, expanded, sort_order
		 FROM groups WHERE path LIKE ? ESCAPE '\' AND path NOT LIKE ? ESCAPE '\'
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
	res, err := conn.Exec(`UPDATE groups SET expanded = ? WHERE path = ?`, v, path)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("set expanded %q: %w", path, sql.ErrNoRows)
	}
	return nil
}

func RenameGroup(conn *sql.DB, path, newName string) error {
	res, err := conn.Exec(`UPDATE groups SET name = ? WHERE path = ?`, newName, path)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("rename group %q: %w", path, sql.ErrNoRows)
	}
	return nil
}

func DeleteGroup(conn *sql.DB, path string) error {
	_, err := conn.Exec(`DELETE FROM groups WHERE path = ?`, path)
	return err
}

func SetGroupConductor(conn *sql.DB, path, sessionID string) error {
	res, err := conn.Exec(`UPDATE groups SET conductor_session_id = ? WHERE path = ?`, sessionID, path)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("set conductor %q: %w", path, sql.ErrNoRows)
	}
	return nil
}

func scanGroups(rows *sql.Rows) ([]Group, error) {
	groups := []Group{}
	for rows.Next() {
		var g Group
		var expanded int
		if err := rows.Scan(&g.Path, &g.Name, &g.DefaultPath, &g.DefaultTool, &g.ConductorSessionID, &expanded, &g.SortOrder); err != nil {
			return nil, err
		}
		g.Expanded = expanded == 1
		groups = append(groups, g)
	}
	return groups, rows.Err()
}
