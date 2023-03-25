import { useState } from "react"
import CreateUserForm from "./components/CreateUserForm"

function App() {
    const [user, setUser] = useState(null)

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
        }).then((res) => res.json()).then(res => {
            setUser(res)
        }).catch(err => console.log(err))
    }

    return (<div className="App">
        <h1>Hello world</h1>
        {user ? <div>Hello {user.username}</div> : <CreateUserForm createUser={createUser} />} 
    </div>)
}


export default App