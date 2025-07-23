// +build ignore

package main

import (
	"fmt"
	"claude-squad/ui"
)

func main() {
	testContent := `## JavaScript Examples

### Basic JavaScript

` + "```javascript" + `
// This is a comment
const greeting = "Hello, World!";
let count = 42;
var oldStyle = true;

// Function declaration
function sayHello(name) {
    console.log('Hello, ' + name + '!');
    return true;
}

// Arrow function
const multiply = (a, b) => a * b;

// Object literal
const person = {
    name: "Alice",
    age: 30,
    greet: function() {
        console.log("Hi!");
    }
};
` + "```" + `

### Advanced Features

` + "```js" + `
// Async/await
async function fetchData() {
    try {
        const response = await fetch('/api/data');
        const data = await response.json();
        return data;
    } catch (error) {
        console.error('Error:', error);
    }
}

// Classes
class Animal {
    constructor(name) {
        this.name = name;
    }
    
    speak() {
        console.log(this.name + ' makes a sound');
    }
}

// Array methods
const numbers = [1, 2, 3, 4, 5];
const doubled = numbers.map(n => n * 2);
const sum = numbers.reduce((a, b) => a + b, 0);

// Destructuring
const { name, age } = person;
const [first, ...rest] = numbers;

// Template literals
const message = ` + "`Welcome ${name}, you are ${age} years old`" + `;

// Conditional operators
const status = age >= 18 ? 'adult' : 'minor';
const value = null ?? 'default';
` + "```" + `

### React Example

` + "```jsx" + `
import React, { useState, useEffect } from 'react';

const TodoList = ({ items }) => {
    const [todos, setTodos] = useState(items);
    const [filter, setFilter] = useState('all');
    
    useEffect(() => {
        document.title = ` + "`Todos: ${todos.length}`" + `;
    }, [todos]);
    
    const addTodo = (text) => {
        setTodos([...todos, { id: Date.now(), text, done: false }]);
    };
    
    return (
        <div className="todo-list">
            <h1>My Todos</h1>
            {todos.map(todo => (
                <TodoItem key={todo.id} todo={todo} />
            ))}
        </div>
    );
};
` + "```" + `

That's all!`

	rendered := ui.RenderMarkdownLight(testContent)
	fmt.Println(rendered)
}