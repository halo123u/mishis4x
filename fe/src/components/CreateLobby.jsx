import { useContext } from 'react'
import { useNavigate } from 'react-router-dom'
import { AuthContext } from '../AuthContext'

const CreateLobby = () => {
  const { user } = useContext(AuthContext)
  const navigate = useNavigate()

  const handleSubmit = (event) => {
    event.preventDefault()
    fetch('/api/lobbies/create', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        name: event.target.name.value,
        user_id: parseInt(user.user_id),
      }),
    })
      .then((res) => res.json())
      .then((response) => {
        console.log(response)
        navigate('/lobbies')
      })
      .catch((err) => {
        console.log(err)
      })
  }

  return (
    <div>
      <form onSubmit={handleSubmit}>
        <label htmlFor="name">name</label>
        <input type="text" id="name" />

        <button type="submit">Submit</button>
      </form>
    </div>
  )
}

export default CreateLobby
