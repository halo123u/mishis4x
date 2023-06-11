import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'

const Lobbies = () => {
  const [lobbies, setLobbies] = useState([])

  useEffect(() => {
    fetch('/api/lobbies', {
      method: 'GET',
    })
      .then((res) => res.json())
      .then((response) => {
        setLobbies(response)
      })
  }, [])

  const joinMatch = () => {
    console.log('joining match')
  }

  return (
    <div>
      <h1>Find or Create Lobby</h1>

      <table>
        <thead>
          <tr>
            <th>name</th>
            <th>players</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {lobbies &&
            lobbies.map((lobby) => (
              <tr key={lobby.Id}>
                <td>{lobby.Name}</td>
                <td>{`${lobby.PlayerIds.length}/2`}</td>
                <td>
                  {lobby.PlayerIds.length < 2 && (
                    <button type="button" onClick={joinMatch}>
                      Join match
                    </button>
                  )}
                </td>
              </tr>
            ))}
        </tbody>
      </table>

      <div>
        <Link to={`/lobbies/create`}>Create Lobby</Link>
      </div>
    </div>
  )
}

export default Lobbies
