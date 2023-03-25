import CreateUserForm from "./components/CreateUserForm"

function App() {

    const createUser = (username, password) => {
        fetch('/api/user/create', {
            method: 'POST',
            headers: {
                "Content-type": "application/json"
            },
            body: JSON.stringify({
                username,
                password
            })
        })
    }

    return (<div className="App">
        <h1>Hello world</h1>
        <CreateUserForm createUser={createUser} />
    </div>)
}


export default App