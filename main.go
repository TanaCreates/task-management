package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	_ "github.com/lib/pq"
)

func main() {
	// Connection string
	connStr := "postgres://postgres:tanaka2004%21@localhost:5432/tms?sslmode=disable"

	// Connect to database
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Error initializing database connection:", err) 
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		log.Fatal("Error pinging the database:", err)
	}
	log.Println("Connected to the database successfully")

	// Create the user and task tables if they don't exist
	createUserTable(db)
	createTaskTable(db)

	// Menu for user to choose options
	for {

		fmt.Println("\n=======Choose an option:==========")
		fmt.Println("1 - Sign Up")
		fmt.Println("2 - Log In")
		fmt.Println("3 - Exit")

		// Reading user choice
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Enter your choice: ")
		choiceInput, _ := reader.ReadString('\n')
		choiceInput = sanitizeInput(choiceInput)
//conversion int>string
		choice, err := strconv.Atoi(choiceInput)
		if err != nil {
			log.Println("Invalid input. Please enter a number.")
			continue
		}

		switch choice {
		case 1:
			signUp(db)
		case 2:
			if loggedInUserID := logIn(db); loggedInUserID > 0 {
				taskMenu(db, loggedInUserID)
			}
		case 3:
			fmt.Println("Exiting program...")
			os.Exit(0)
		default:
			fmt.Println("Invalid choice. Please try again.")
		}
	}
}

// create the "user" table
func createUserTable(db *sql.DB) {
	query := `CREATE TABLE IF NOT EXISTS "user" (
		user_id SERIAL PRIMARY KEY,
		username VARCHAR(50) UNIQUE NOT NULL
	)`
	_, err := db.Exec(query)
	if err != nil {
		log.Fatal("Error creating user table:", err)
	}
}

//create the "task" table
func createTaskTable(db *sql.DB) {
	query := `CREATE TABLE IF NOT EXISTS "task"(
		task_id SERIAL PRIMARY KEY,
		user_id INT NOT NULL REFERENCES "user"(user_id),
		title VARCHAR(50) NOT NULL,
		description TEXT
	)`
	_, err := db.Exec(query)
	if err != nil {
		log.Fatal("Error creating task table:", err)
	}
}

func signUp(db *sql.DB) {
	reader := bufio.NewReader(os.Stdin)

	// Prompt for username
	fmt.Print("Enter username: ")
	username, err := reader.ReadString('\n')
	if err != nil {
		log.Println("Error reading input:", err)
		return
	}

	// Sanitize input
	username = sanitizeInput(username)

	// Insert into the database
	query := `INSERT INTO "user" (username) VALUES ($1)`
	_, err = db.Exec(query, username)
	if err != nil {
		log.Println("Error signing up:", err)
		fmt.Println("Username might already exist. Try again.")
		return
	}
	fmt.Println("Sign Up successful! You can now log in.")
}

func logIn(db *sql.DB) int {
	reader := bufio.NewReader(os.Stdin)

	// Prompt for username
	fmt.Print("Enter username: ")
	username, err := reader.ReadString('\n')
	if err != nil {
		log.Println("Error reading input:", err)
		return 0
	}

	username = sanitizeInput(username)

	// Query the database to check if the username exists
	query := `SELECT user_id FROM "user" WHERE username = $1`
	row := db.QueryRow(query, username)

	var userID int
	err = row.Scan(&userID)
	if err == sql.ErrNoRows {
		fmt.Println("Invalid username. Please sign up first.")
		return 0
	} else if err != nil {
		log.Println("Error logging in:", err)
		return 0
	}

	fmt.Printf("Welcome back, %s! You are logged in.\n", username)
	return userID
}

// Task management menu
func taskMenu(db *sql.DB, userID int) {
	for {
		fmt.Println("\nTask Management Menu:")
		fmt.Println("---------------------------------")
		fmt.Println("1 - Create Task")
		fmt.Println("2 - View Tasks")
		fmt.Println("3 - Update Task")
		fmt.Println("4 - Delete Task")
		fmt.Println("5 - Logout")

		var choice int
		fmt.Print("Enter your choice: ")
		_, err := fmt.Scan(&choice)
		if err != nil {
			log.Println("Invalid input. Please enter a number.")
			continue
		}

		switch choice {
		case 1:
			createTask(db, userID)
		case 2:
			viewTasks(db, userID)
		case 3:
			updateTask(db, userID)
		case 4:
			deleteTask(db, userID)
		case 5:
			fmt.Println("Logging out...")
			return
		default:
			fmt.Println("Invalid choice. Please try again.")
		}
	}
}

func createTask(db *sql.DB, userID int) {
	reader := bufio.NewReader(os.Stdin)

	// Clear buffer
	reader.ReadString('\n')

	fmt.Print("Enter title: ")
	title, err := reader.ReadString('\n')
	if err != nil {
		log.Println("Error reading input:", err)
		return
	}
	title = sanitizeInput(strings.TrimSpace(title))

	fmt.Print("Enter description: ")
	description, err := reader.ReadString('\n')
	if err != nil {
		log.Println("Error reading input:", err)
		return
	}
	description = sanitizeInput(strings.TrimSpace(description))

	// Insert task into the database
	query := `INSERT INTO "task" (user_id, title, description) VALUES ($1, $2, $3)`
	_, execErr := db.Exec(query, userID, title, description)
	if execErr != nil {
		log.Println("Error creating task:", execErr)
		return
	}
	fmt.Println("Task created successfully!")
}

func viewTasks(db *sql.DB, userID int) {
	query := `SELECT task_id, title, description FROM "task" WHERE user_id = $1`
	rows, err := db.Query(query, userID)
	if err != nil {
		log.Println("Error retrieving tasks:", err)
		return
	}
	defer rows.Close()

	fmt.Println("Your Tasks:")
	for rows.Next() {
		var taskID int
		var title, description string
		rows.Scan(&taskID, &title, &description)
		fmt.Printf("ID: %d, Title: %s, Description: %s\n", taskID, title, description)
	}
}

func updateTask(db *sql.DB, userID int) {
	reader := bufio.NewReader(os.Stdin)
	reader.ReadString('\n')

	fmt.Print("Enter task ID to update: ")
	taskIDInput, _ := reader.ReadString('\n')
	taskID, _ := strconv.Atoi(sanitizeInput(taskIDInput))

	fmt.Print("Enter new task title: ")
	title, _ := reader.ReadString('\n')
	title = sanitizeInput(title)

	fmt.Print("Enter new task description: ")
	description, _ := reader.ReadString('\n')
	description = sanitizeInput(description)

	query := `UPDATE "task" SET title = $1, description = $2 WHERE task_id = $3 AND user_id = $4`
	_, err := db.Exec(query, title, description, taskID, userID)
	if err != nil {
		log.Println("Error updating task:", err)
		return
	}
	fmt.Println("Task updated successfully!")
}

func deleteTask(db *sql.DB, userID int) {
	reader := bufio.NewReader(os.Stdin)
	reader.ReadString('\n') // Discard leftover newline from the menu choice

	fmt.Print("Enter task ID to delete: ")
	taskIDInput, _ := reader.ReadString('\n')
	taskID, _ := strconv.Atoi(sanitizeInput(taskIDInput))

	query := `DELETE FROM "task" WHERE task_id = $1 AND user_id = $2`
	_, err := db.Exec(query, taskID, userID)
	if err != nil {
		log.Println("Error deleting task:", err)
		return
	}
	fmt.Println("Task deleted successfully!")
}

// Helper function to sanitize user input
func sanitizeInput(input string) string {
	return strings.TrimSpace(input)
}
